package engine

import "context"

type MultiLimiter struct {
	limiters []*RateLimiter
}

func NewMultiLimiter(limiters ...*RateLimiter) *MultiLimiter {
	filtered := make([]*RateLimiter, 0, len(limiters))
	for _, l := range limiters {
		if l != nil {
			filtered = append(filtered, l)
		}
	}
	return &MultiLimiter{limiters: filtered}
}

func (m *MultiLimiter) WaitN(ctx context.Context, n int64) error {
	if m == nil {
		return nil
	}
	for i, l := range m.limiters {
		if err := l.WaitN(ctx, n); err != nil {
			for j := 0; j < i; j++ {
				m.limiters[j].Refund(n)
			}
			return err
		}
	}
	return nil
}
