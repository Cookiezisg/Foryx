package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
)

// First runs the query with LIMIT 1 and returns the single row, or ErrNotFound.
//
// First 以 LIMIT 1 执行并返回单行，无则返 ErrNotFound。
func (q *Query[T]) First(ctx context.Context) (*T, error) {
	q.limit = 1
	stmt, args, err := q.buildSelect(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.db.handle().QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("orm: first: %w", err)
	}
	defer rows.Close()
	out, err := scanAll[T](rows, q.meta)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out[0], nil
}

// Find runs the query and returns all matching rows (possibly empty).
//
// Find 执行查询并返回所有匹配行（可能为空）。
func (q *Query[T]) Find(ctx context.Context) ([]*T, error) {
	stmt, args, err := q.buildSelect(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.db.handle().QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("orm: find: %w", err)
	}
	defer rows.Close()
	return scanAll[T](rows, q.meta)
}

// Count returns the number of matching rows (ignores limit/offset/order).
//
// Count 返回匹配行数（忽略 limit/offset/order）。
func (q *Query[T]) Count(ctx context.Context) (int64, error) {
	where, args, err := q.whereClause(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	row := q.db.handle().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+q.table+where, args...)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("orm: count: %w", err)
	}
	return n, nil
}

// Exists reports whether any row matches.
//
// Exists 报告是否存在匹配行。
func (q *Query[T]) Exists(ctx context.Context) (bool, error) {
	where, args, err := q.whereClause(ctx)
	if err != nil {
		return false, err
	}
	var one int
	row := q.db.handle().QueryRowContext(ctx, "SELECT 1 FROM "+q.table+where+" LIMIT 1", args...)
	switch err := row.Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("orm: exists: %w", err)
	default:
		return true, nil
	}
}

// Pluck scans a single column from every matching row into dst, which must be a
// pointer to a slice (e.g. *[]string). Honors WHERE / Order / Limit.
//
// Pluck 把每个匹配行的某一列扫进 dst（须为指向切片的指针，如 *[]string）。遵循 WHERE / Order / Limit。
func (q *Query[T]) Pluck(ctx context.Context, col string, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("orm: Pluck dst must be a pointer to a slice, got %T", dst)
	}
	where, args, err := q.whereClause(ctx)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT %s FROM %s%s", col, q.table, where)
	if q.order != "" {
		b.WriteString(" ORDER BY " + q.order)
	}
	if q.limit > 0 {
		fmt.Fprintf(&b, " LIMIT %d", q.limit)
	}

	rows, err := q.db.handle().QueryContext(ctx, b.String(), args...)
	if err != nil {
		return fmt.Errorf("orm: pluck: %w", err)
	}
	defer rows.Close()

	slice := rv.Elem()
	elemType := slice.Type().Elem()
	for rows.Next() {
		elem := reflect.New(elemType)
		if err := rows.Scan(elem.Interface()); err != nil {
			return fmt.Errorf("orm: pluck scan: %w", err)
		}
		slice = reflect.Append(slice, elem.Elem())
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: pluck rows: %w", err)
	}
	rv.Elem().Set(slice)
	return nil
}

// Page returns one keyset page ordered by (keyset, pk) descending, where the keyset column is the
// created column by default or whatever PageKeyset set (e.g. last_message_at). It applies the tuple
// cursor, fetches limit+1 rows to detect a next page, trims to limit, and returns the next cursor
// ("" when exhausted). Requires the keyset (created by default) + pk columns. limit <= 0 → 50.
//
// Page 返回一页 keyset 结果，按 (keyset, pk) 降序——keyset 列默认 created 列，或 PageKeyset 指定的列
// （如 last_message_at）。套用元组游标、多取一行探测下页、裁剪到 limit、返回下一页游标（取尽为 ""）。
// 需 keyset（默认 created）+ pk 列。limit <= 0 取 50。
func (q *Query[T]) Page(ctx context.Context, cursor string, limit int) ([]*T, string, error) {
	if limit <= 0 {
		limit = 50
	}
	ks := q.keyset
	if ks == nil {
		ks = q.meta.created
	}
	if ks == nil || q.meta.pk == nil {
		return nil, "", fmt.Errorf("orm: Page requires a keyset (created or PageKeyset) + pk column on %s", q.table)
	}

	if cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(cursor, &c); err != nil {
			return nil, "", fmt.Errorf("orm: page cursor: %w", err)
		}
		q.Where("("+ks.name+", "+q.meta.pk.name+") < (?, ?)", c.Key, c.ID)
	}
	// Default keyset order is (keyset, pk) DESC — matching the cursor's tuple comparison. A caller
	// MAY prepend a leading clause via a prior .Order() (kept intentionally): conversation lists are
	// pinned-first ("pinned DESC, last_message_at DESC, id DESC"). The cursor keys only (keyset, pk),
	// so the leading pinned partition relies on all pins landing on page one (few, single-user) —
	// that assumption, not the default order, is what makes pinned-first safe. Whatever column the
	// .Order() sorts by MUST match PageKeyset, or pages skip/duplicate rows.
	//
	// 默认 keyset 排序 (keyset, pk) DESC，与游标元组比较一致。调用方可用先前 .Order() 前置一个引导子句
	// （有意保留）：conversation 置顶优先（"pinned DESC, last_message_at DESC, id DESC"）。游标只键
	// (keyset, pk)，故置顶分区靠"所有置顶都落首页"（少、单用户）——是这个假设而非默认序让置顶优先安全。
	// .Order() 所按的列必须与 PageKeyset 一致，否则跨页漏行/重行。
	if q.order == "" {
		q.order = ks.name + " DESC, " + q.meta.pk.name + " DESC"
	}
	q.limit = limit + 1

	rows, err := q.Find(ctx)
	if err != nil {
		return nil, "", err
	}

	var next string
	if len(rows) > limit {
		last := reflect.ValueOf(rows[limit-1]).Elem()
		key, _ := last.Field(ks.index).Interface().(time.Time)
		id, _ := last.Field(q.meta.pk.index).Interface().(string)
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{Key: key, ID: id})
		if err != nil {
			return nil, "", fmt.Errorf("orm: page cursor encode: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}
