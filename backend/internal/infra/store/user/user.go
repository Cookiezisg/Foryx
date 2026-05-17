// Package user is the GORM-backed implementation of userdomain.Repository.
//
// Package user 是 userdomain.Repository 的 GORM 实现。
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
)

// Store is the GORM-backed userdomain.Repository.
//
// Store 是 userdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Save(ctx context.Context, u *userdomain.User) error {
	if err := s.db.WithContext(ctx).Save(u).Error; err != nil {
		// SQLite UNIQUE-constraint message matching keeps the contract self-contained.
		// SQLite UNIQUE 约束触发时翻译到 ErrUsernameConflict。
		if isUniqueConstraint(err) {
			return userdomain.ErrUsernameConflict
		}
		return fmt.Errorf("userstore.Save: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*userdomain.User, error) {
	var u userdomain.User
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, userdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("userstore.Get: %w", err)
	}
	return &u, nil
}

func (s *Store) GetByUsername(ctx context.Context, username string) (*userdomain.User, error) {
	var u userdomain.User
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, userdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("userstore.GetByUsername: %w", err)
	}
	return &u, nil
}

func (s *Store) List(ctx context.Context) ([]*userdomain.User, error) {
	var rows []*userdomain.User
	err := s.db.WithContext(ctx).Order("created_at ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("userstore.List: %w", err)
	}
	return rows, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&userdomain.User{})
	if res.Error != nil {
		return fmt.Errorf("userstore.Delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return userdomain.ErrNotFound
	}
	return nil
}

func (s *Store) Count(ctx context.Context) (int, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&userdomain.User{}).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("userstore.Count: %w", err)
	}
	return int(n), nil
}

func (s *Store) TouchLastUsed(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&userdomain.User{}).
		Where("id = ?", id).Update("last_used_at", now)
	if res.Error != nil {
		return fmt.Errorf("userstore.TouchLastUsed: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return userdomain.ErrNotFound
	}
	return nil
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// modernc.org/sqlite returns "constraint failed: UNIQUE constraint failed: ..." or similar.
	// modernc.org/sqlite UNIQUE 违规会出现 "UNIQUE constraint failed" 字样。
	return contains(msg, "UNIQUE constraint failed")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
