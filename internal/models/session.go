package models

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionDuration = 24 * time.Hour

type Session struct {
	ID        int64
	Token     string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func CreateSession(ctx context.Context, db *pgxpool.Pool, userID int64) (Session, error) {
	token, err := generateToken()
	if err != nil {
		return Session{}, err
	}

	expiresAt := time.Now().Add(sessionDuration)

	var id int64
	err = queryRow(ctx, db, sqlCreateSession, token, userID, expiresAt).Scan(&id)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}

	return Session{
		ID:        id,
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}, nil
}

func GetSessionByToken(ctx context.Context, db *pgxpool.Pool, token string) (Session, error) {
	rows, err := queryRows(ctx, db, sqlGetSessionByToken, token)
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Session])
}

func DeleteSession(ctx context.Context, db *pgxpool.Pool, token string) error {
	_, err := execSQL(ctx, db, sqlDeleteSession, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func DeleteExpiredSessions(ctx context.Context, db *pgxpool.Pool) error {
	_, err := execSQL(ctx, db, sqlDeleteExpiredSessions)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
