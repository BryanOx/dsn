package sdk

const (
	// DefaultLimit is the default page size.
	DefaultLimit = 20

	// MaxLimit is the maximum allowed page size.
	MaxLimit = 100
)

// Pagination holds pagination parameters.
type Pagination struct {
	Offset uint64 `json:"offset"`
	Limit  uint64 `json:"limit"`
}

// DefaultPagination returns default pagination settings.
func DefaultPagination() Pagination {
	return Pagination{
		Offset: 0,
		Limit:  DefaultLimit,
	}
}

// Normalize caps limit at MaxLimit and ensures defaults.
func (p *Pagination) Normalize() {
	if p.Limit == 0 {
		p.Limit = DefaultLimit
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	if p.Offset > 0 && p.Limit == 0 {
		p.Limit = DefaultLimit
	}
}

// HasMore indicates if there are more results.
func (p *Pagination) HasMore(total uint64) bool {
	return p.Offset+p.Limit < total
}

// Next returns the next pagination params.
func (p *Pagination) Next() Pagination {
	return Pagination{
		Offset: p.Offset + p.Limit,
		Limit:  p.Limit,
	}
}

// PaginationOption functional option for pagination.
type PaginationOption func(*Pagination)

// WithOffset sets the offset.
func WithOffset(offset uint64) PaginationOption {
	return func(p *Pagination) {
		p.Offset = offset
	}
}

// WithLimit sets the limit.
func WithLimit(limit uint64) PaginationOption {
	return func(p *Pagination) {
		p.Limit = limit
	}
}

// ApplyOptions applies pagination options.
func ApplyOptions(opts ...PaginationOption) Pagination {
	p := DefaultPagination()
	for _, opt := range opts {
		opt(&p)
	}
	p.Normalize()
	return p
}

// PageInfo holds pagination response info.
type PageInfo struct {
	Offset      uint64 `json:"offset"`
	Limit       uint64 `json:"limit"`
	Total       uint64 `json:"total"`
	HasNextPage bool   `json:"hasNextPage"`
	HasPrevPage bool   `json:"hasPrevPage"`
}

// NewPageInfo creates new page info from pagination and total.
func NewPageInfo(p Pagination, total uint64) PageInfo {
	return PageInfo{
		Offset:      p.Offset,
		Limit:       p.Limit,
		Total:       total,
		HasNextPage: p.HasMore(total),
		HasPrevPage: p.Offset > 0,
	}
}

// PaginatedResponse wraps a response with pagination info.
type PaginatedResponse[T any] struct {
	Data     []T      `json:"data"`
	PageInfo PageInfo `json:"pageInfo"`
}

// NewPaginatedResponse creates a new paginated response.
func NewPaginatedResponse[T any](data []T, p Pagination, total uint64) PaginatedResponse[T] {
	return PaginatedResponse[T]{
		Data:     data,
		PageInfo: NewPageInfo(p, total),
	}
}
