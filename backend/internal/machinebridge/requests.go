package machinebridge

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acpcredentials"
)

type outboundRequests struct {
	ctx     context.Context
	write   func(Frame) error
	cancel  context.CancelFunc
	mu      sync.Mutex
	next    uint64
	pending map[string]chan Frame
}

func (r *outboundRequests) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	r.mu.Lock()
	r.next++
	id := "client-" + strconv.FormatUint(r.next, 10)
	in := make(chan Frame, 1)
	r.pending[id] = in
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
	}()
	if r.ctx.Err() != nil {
		return nil, ErrDeviceOffline
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.write(Frame{ID: id, Method: method, Params: params}); err != nil {
		r.cancel()
		return nil, ErrDeviceOffline
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.ctx.Done():
		return nil, ErrDeviceOffline
	case response := <-in:
		if r.ctx.Err() != nil {
			return nil, ErrDeviceOffline
		}
		if response.Error != nil {
			return nil, response.Error
		}
		return response.Result, nil
	}
}

func (r *outboundRequests) reply(frame Frame) {
	r.mu.Lock()
	in := r.pending[frame.ID]
	delete(r.pending, frame.ID)
	r.mu.Unlock()
	if in != nil {
		in <- frame
	}
}

func (c *Client) ResolveACPProfile(ctx context.Context, profileID string, rejectedVersion int64) (acpcredentials.Profile, error) {
	c.mu.RLock()
	requests, negotiated := c.requests, c.negotiated.ACPProfiles
	c.mu.RUnlock()
	if requests == nil {
		return acpcredentials.Profile{}, ErrDeviceOffline
	}
	if !negotiated {
		return acpcredentials.Profile{}, ErrUnsupportedCapability
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	params, err := json.Marshal(struct {
		ID              string `json:"profile_id"`
		RejectedVersion int64  `json:"rejected_version,omitempty"`
	}{profileID, rejectedVersion})
	if err != nil {
		return acpcredentials.Profile{}, err
	}
	for {
		raw, err := requests.call(ctx, methodACPProfileResolve, params)
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == CodeConflict {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return acpcredentials.Profile{}, ctx.Err()
			case <-timer.C:
				continue
			}
		}
		if err != nil {
			return acpcredentials.Profile{}, err
		}
		var profile acpcredentials.Profile
		if json.Unmarshal(raw, &profile) != nil || profile.ID != profileID || profile.Version <= 0 {
			return acpcredentials.Profile{}, errors.New("invalid Cloud ACP profile response")
		}
		return profile, nil
	}
}
