package catalog

import "context"

// Granularity tells the Generator how aggressively it may merge a source's items.
//
// Granularity 控制 Generator 合并 source items 的激进程度。
type Granularity int

const (
	PerItem Granularity = iota
	PerServer
)

func (g Granularity) String() string {
	switch g {
	case PerItem:
		return "PerItem"
	case PerServer:
		return "PerServer"
	default:
		return "Unknown"
	}
}

// Item is one entry in a source's ListItems return; Source+ID must uniquely identify it.
//
// Item 是 source ListItems 返回的一条；Source+ID 在整 catalog 内须唯一。
type Item struct {
	Source      string `json:"source"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

// CatalogSource is what every capability provider implements to join the catalog.
//
// CatalogSource 是所有能力提供方为参与 catalog 而实现的接口。
type CatalogSource interface {
	Name() string
	Granularity() Granularity

	// ListItems returns current truth; errors substitute an empty list for this tick.
	//
	// ListItems 返当前真实状态；出错时本 tick 该 source 用空列表替代。
	ListItems(ctx context.Context) ([]Item, error)
}
