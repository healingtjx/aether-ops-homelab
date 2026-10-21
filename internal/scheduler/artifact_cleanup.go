package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"aether_ops/internal/models"
	"aether_ops/internal/registry"
)

const (
	artifactCleanupCron      = "0 3 * * *"
	artifactRetentionPerRepo = 3
	artifactCleanupTimeout   = 30 * time.Minute
)

func Start(pool *pgxpool.Pool) *cron.Cron {
	c := cron.New()
	var cleanupMu sync.Mutex
	run := func() {
		if !cleanupMu.TryLock() {
			log.Printf("artifact cleanup: skip (already running)")
			return
		}
		defer cleanupMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), artifactCleanupTimeout)
		defer cancel()
		runArtifactCleanup(ctx, pool)
	}
	_, err := c.AddFunc(artifactCleanupCron, run)
	if err != nil {
		log.Printf("scheduler: register artifact cleanup failed: %v", err)
		return c
	}
	c.Start()
	log.Printf("scheduler: artifact cleanup scheduled (cron=%q, keep_latest=%d)", artifactCleanupCron, artifactRetentionPerRepo)
	return c
}

func runArtifactCleanup(ctx context.Context, db *pgxpool.Pool) {
	start := time.Now()
	log.Printf("artifact cleanup: start")

	registries, err := models.ListArtifactRegistries(ctx, db)
	if err != nil {
		log.Printf("artifact cleanup: list registries failed: %v", err)
		return
	}

	totalDeleted, totalFailed := 0, 0
	for _, reg := range registries {
		if !reg.Enabled {
			continue
		}
		deleted, failed := cleanupRegistry(ctx, db, reg)
		totalDeleted += deleted
		totalFailed += failed
	}

	log.Printf("artifact cleanup: done in %s, deleted=%d, failed=%d", time.Since(start).Round(time.Second), totalDeleted, totalFailed)
}

func cleanupRegistry(ctx context.Context, db *pgxpool.Pool, reg models.ArtifactRegistry) (int, int) {
	opLog, _ := models.CreateArtifactOperationLog(ctx, db, reg.ID, 0, "cleanup")
	finish := func(status, msg string) {
		if opLog.ID > 0 {
			models.UpdateArtifactOperationLogStatus(ctx, db, opLog.ID, status, msg)
		}
	}

	client, err := registry.NewClient(registry.Config{
		RegistryURL: reg.RegistryURL,
		Namespace:   reg.Namespace,
		Provider:    reg.Provider,
		AuthType:    registry.AuthType(reg.AuthType),
		Username:    reg.Username,
		Credential:  reg.Credential,
	})
	if err != nil {
		log.Printf("artifact cleanup: registry %q new client failed: %v", reg.Name, err)
		finish("failed", err.Error())
		return 0, 0
	}

	// 同步远端镜像到 DB，确保数据最新
	var images []registry.Image
	if len(reg.RepositoryList()) > 0 {
		var result registry.ListResult
		result, err = client.ListRepositoryImagesResult(ctx, reg.RepositoryList())
		if err == nil && len(result.Errors) > 0 {
			err = fmt.Errorf("仓库 %s 同步失败: %w", result.Errors[0].Repository, result.Errors[0].Err)
		}
		images = result.Images
	} else {
		images, err = client.ListImages(ctx)
	}
	if err != nil {
		log.Printf("artifact cleanup: registry %q sync failed: %v", reg.Name, err)
		finish("failed", err.Error())
		return 0, 0
	}
	items := make([]models.ArtifactSyncItem, 0, len(images))
	for _, image := range images {
		items = append(items, models.ArtifactSyncItem{
			Repository: image.Repository,
			Tag:        image.Tag,
			Digest:     image.Digest,
			MediaType:  image.MediaType,
			SizeBytes:  image.SizeBytes,
			PushedAt:   image.PushedAt,
		})
	}
	if err := models.SyncArtifacts(ctx, db, reg.ID, items); err != nil {
		log.Printf("artifact cleanup: registry %q sync db failed: %v", reg.Name, err)
		finish("failed", err.Error())
		return 0, 0
	}
	log.Printf("artifact cleanup: registry %q synced %d images", reg.Name, len(items))

	artifacts, err := models.ListArtifacts(ctx, db, reg.ID, "present")
	if err != nil {
		log.Printf("artifact cleanup: registry %q list artifacts failed: %v", reg.Name, err)
		finish("failed", err.Error())
		return 0, 0
	}

	groups := groupByRepository(artifacts)
	deleted, failed := 0, 0
	for repo, items := range groups {
		sortArtifactsByPushedDesc(items)
		if len(items) <= artifactRetentionPerRepo {
			continue
		}
		victims := items[artifactRetentionPerRepo:]
		// 从最旧的开始删除（反转，使日志按时间正序）
		for i, j := 0, len(victims)-1; i < j; i, j = i+1, j-1 {
			victims[i], victims[j] = victims[j], victims[i]
		}
		for _, victim := range victims {
			digest := ""
			if victim.Digest != nil {
				digest = *victim.Digest
			}
			time.Sleep(2 * time.Second)
			if err := client.DeleteManifest(ctx, victim.Repository, digest, victim.Tag); err != nil {
				log.Printf("artifact cleanup: delete %s:%s remote failed: %v", repo, victim.Tag, err)
				failed++
				continue
			}
			if err := models.MarkArtifactDeleted(ctx, db, victim.ID); err != nil {
				log.Printf("artifact cleanup: mark %s:%s deleted failed: %v", repo, victim.Tag, err)
				failed++
				continue
			}
			deleted++
			log.Printf("artifact cleanup: deleted %s:%s", repo, victim.Tag)
		}
	}

	msg := fmt.Sprintf("清理完成，删除 %d 个 tag", deleted)
	status := "success"
	if failed > 0 {
		status = "failed"
		msg = fmt.Sprintf("%s，失败 %d 个", msg, failed)
	}
	finish(status, msg)
	return deleted, failed
}

func groupByRepository(artifacts []models.Artifact) map[string][]models.Artifact {
	groups := make(map[string][]models.Artifact)
	for _, a := range artifacts {
		groups[a.Repository] = append(groups[a.Repository], a)
	}
	return groups
}

func sortArtifactsByPushedDesc(items []models.Artifact) {
	sort.SliceStable(items, func(i, j int) bool {
		left := artifactSortTime(items[i])
		right := artifactSortTime(items[j])
		if left.Equal(right) {
			return items[i].Tag > items[j].Tag
		}
		return left.After(right)
	})
}

func artifactSortTime(a models.Artifact) time.Time {
	if a.PushedAt != nil {
		return *a.PushedAt
	}
	if a.LastSyncedAt != nil {
		return *a.LastSyncedAt
	}
	return a.CreatedAt
}
