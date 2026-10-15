package models

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func CreateUser(ctx context.Context, db *pgxpool.Pool, username string, passwordHash string) (User, error) {
	var id int64
	err := queryRow(ctx, db, sqlCreateUser, username, passwordHash).Scan(&id)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return GetUserByID(ctx, db, id)
}

func GetUserByID(ctx context.Context, db *pgxpool.Pool, id int64) (User, error) {
	rows, err := queryRows(ctx, db, sqlGetUserByID, id)
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[User])
}

func GetUserByUsername(ctx context.Context, db *pgxpool.Pool, username string) (User, error) {
	rows, err := queryRows(ctx, db, sqlGetUserByUsername, username)
	if err != nil {
		return User{}, fmt.Errorf("get user by username: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[User])
}
