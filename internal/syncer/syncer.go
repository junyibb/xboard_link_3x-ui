package syncer

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"xboard_link_3x-ui/internal/config"
	"xboard_link_3x-ui/internal/state"
	"xboard_link_3x-ui/internal/xboard"
	"xboard_link_3x-ui/internal/xui"
)

type XboardClient interface {
	Users(ctx context.Context, etag string) ([]xboard.User, string, bool, error)
	PushTraffic(ctx context.Context, traffic map[int64][2]uint64) error
}

type XUIClient interface {
	ListClients(ctx context.Context) ([]xui.ManagedClient, error)
	AddClient(ctx context.Context, client xui.ManagedClient, inboundIDs []int) error
	UpdateClient(ctx context.Context, oldEmail string, client xui.ManagedClient) error
	DeleteClient(ctx context.Context, email string, keepTraffic bool) error
	AttachClient(ctx context.Context, email string, inboundIDs []int) error
	DetachClient(ctx context.Context, email string, inboundIDs []int) error
	Traffic(ctx context.Context, email string) (xui.TrafficInfo, error)
}

type Service struct {
	cfg      *config.Config
	xb       XboardClient
	xu       XUIClient
	state    *state.Store
	userETag string
}

func New(cfg *config.Config, xb XboardClient, xu XUIClient, store *state.Store) *Service {
	return &Service{cfg: cfg, xb: xb, xu: xu, state: store}
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.RunOnce(ctx, true); err != nil {
		log.Printf("initial sync failed: %v", err)
	}

	syncTicker := time.NewTicker(s.cfg.Sync.Interval())
	defer syncTicker.Stop()

	trafficTicker := time.NewTicker(s.cfg.Sync.TrafficInterval())
	defer trafficTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-syncTicker.C:
			if err := s.RunOnce(ctx, false); err != nil {
				log.Printf("sync failed: %v", err)
			}
		case <-trafficTicker.C:
			if s.cfg.Sync.EnableTrafficReport {
				if err := s.ReportTraffic(ctx); err != nil {
					log.Printf("traffic report failed: %v", err)
				}
			}
		}
	}
}

func (s *Service) RunOnce(ctx context.Context, includeTraffic bool) error {
	users, etag, notModified, err := s.xb.Users(ctx, s.userETag)
	if err != nil {
		return err
	}
	if etag != "" {
		s.userETag = etag
	}

	if notModified {
		log.Printf("xboard users not modified")
	} else {
		if err := s.SyncUsers(ctx, users); err != nil {
			return err
		}
		if err := s.state.Save(); err != nil {
			return err
		}
	}

	if includeTraffic && s.cfg.Sync.EnableTrafficReport {
		return s.ReportTraffic(ctx)
	}
	return nil
}

func (s *Service) SyncUsers(ctx context.Context, users []xboard.User) error {
	targetInboundIDs := s.cfg.TargetInboundIDs()
	existing, err := s.xu.ListClients(ctx)
	if err != nil {
		return err
	}

	existingByEmail := make(map[string]xui.ManagedClient, len(existing))
	for _, client := range existing {
		existingByEmail[client.Email] = client
	}

	desired := make(map[string]xui.ManagedClient, len(users))
	for _, user := range users {
		if user.ID <= 0 || strings.TrimSpace(user.UUID) == "" {
			log.Printf("skip invalid xboard user: id=%d uuid=%q", user.ID, user.UUID)
			continue
		}
		client := s.clientForUser(user)
		desired[client.Email] = client
	}

	created, updated, attached, detached, deleted := 0, 0, 0, 0, 0
	for email, want := range desired {
		have, ok := existingByEmail[email]
		if !ok {
			if err := s.xu.AddClient(ctx, want, targetInboundIDs); err != nil {
				return fmt.Errorf("add client %s: %w", email, err)
			}
			created++
			continue
		}

		if clientNeedsUpdate(have, want) {
			if err := s.xu.UpdateClient(ctx, email, want); err != nil {
				return fmt.Errorf("update client %s: %w", email, err)
			}
			updated++
		}

		toAttach, toDetach := inboundDiff(have.InboundIDs, targetInboundIDs)
		if len(toAttach) > 0 {
			if err := s.xu.AttachClient(ctx, email, toAttach); err != nil {
				return fmt.Errorf("attach client %s: %w", email, err)
			}
			attached += len(toAttach)
		}
		if len(toDetach) > 0 {
			if err := s.xu.DetachClient(ctx, email, toDetach); err != nil {
				return fmt.Errorf("detach client %s: %w", email, err)
			}
			detached += len(toDetach)
		}
	}

	if s.cfg.Sync.DeleteStale {
		for _, have := range existing {
			if !s.isManagedEmail(have.Email) {
				continue
			}
			if _, ok := desired[have.Email]; ok {
				continue
			}
			if err := s.xu.DeleteClient(ctx, have.Email, false); err != nil {
				return fmt.Errorf("delete stale client %s: %w", have.Email, err)
			}
			delete(s.state.Data.Traffic, have.Email)
			deleted++
		}
	}

	log.Printf("sync users done: desired=%d created=%d updated=%d attached=%d detached=%d deleted=%d", len(desired), created, updated, attached, detached, deleted)
	return nil
}

func (s *Service) ReportTraffic(ctx context.Context) error {
	clients, err := s.xu.ListClients(ctx)
	if err != nil {
		return err
	}

	report := make(map[int64][2]uint64)
	nextSnapshots := make(map[string]state.TrafficSnapshot)

	for _, client := range clients {
		if !s.isManagedEmail(client.Email) {
			continue
		}
		userID, ok := s.userIDFromEmail(client.Email)
		if !ok {
			continue
		}

		traffic := client.Traffic
		if traffic.Up == 0 && traffic.Down == 0 {
			fresh, err := s.xu.Traffic(ctx, client.Email)
			if err != nil {
				return fmt.Errorf("read traffic %s: %w", client.Email, err)
			}
			traffic = fresh
		}

		prev := s.state.Data.Traffic[client.Email]
		var upDelta, downDelta uint64
		if traffic.Up >= prev.Up {
			upDelta = traffic.Up - prev.Up
		}
		if traffic.Down >= prev.Down {
			downDelta = traffic.Down - prev.Down
		}
		if upDelta > 0 || downDelta > 0 {
			report[userID] = [2]uint64{upDelta, downDelta}
		}

		nextSnapshots[client.Email] = state.TrafficSnapshot{
			UserID: userID,
			Up:     traffic.Up,
			Down:   traffic.Down,
		}
	}

	if len(report) > 0 {
		if err := s.xb.PushTraffic(ctx, report); err != nil {
			return err
		}
	}

	for email, snapshot := range nextSnapshots {
		s.state.Data.Traffic[email] = snapshot
	}
	if err := s.state.Save(); err != nil {
		return err
	}

	log.Printf("traffic report done: users=%d", len(report))
	return nil
}

func (s *Service) clientForUser(user xboard.User) xui.ManagedClient {
	uuid := strings.TrimSpace(user.UUID)
	email := s.emailForUser(user.ID)
	return xui.ManagedClient{
		Email:      email,
		SubID:      uuid,
		UUID:       uuid,
		Password:   uuid,
		TotalGB:    0,
		ExpiryTime: 0,
		Enable:     true,
		TgID:       0,
		LimitIP:    user.DeviceLimit,
		Comment:    fmt.Sprintf("managed by xboard_link_3x-ui user_id=%d", user.ID),
		InboundIDs: append([]int(nil), s.cfg.TargetInboundIDs()...),
	}
}

func (s *Service) emailForUser(userID int64) string {
	return fmt.Sprintf("%s-%s-%d-%d", s.cfg.Sync.EmailPrefix, s.cfg.Xboard.NodeType, s.cfg.Xboard.NodeID, userID)
}

func (s *Service) isManagedEmail(email string) bool {
	prefix := fmt.Sprintf("%s-%s-%d-", s.cfg.Sync.EmailPrefix, s.cfg.Xboard.NodeType, s.cfg.Xboard.NodeID)
	return strings.HasPrefix(email, prefix)
}

func (s *Service) userIDFromEmail(email string) (int64, bool) {
	if !s.isManagedEmail(email) {
		return 0, false
	}
	parts := strings.Split(email, "-")
	if len(parts) == 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	return id, err == nil && id > 0
}

func clientNeedsUpdate(have, want xui.ManagedClient) bool {
	if have.UUID != "" && have.UUID != want.UUID {
		return true
	}
	if have.Password != "" && have.Password != want.Password {
		return true
	}
	if have.TotalGB != want.TotalGB {
		return true
	}
	if have.ExpiryTime != want.ExpiryTime {
		return true
	}
	if have.Enable != want.Enable {
		return true
	}
	if have.LimitIP != want.LimitIP {
		return true
	}
	return false
}

func inboundDiff(have, want []int) ([]int, []int) {
	haveSet := intSet(have)
	wantSet := intSet(want)

	var toAttach []int
	for id := range wantSet {
		if _, ok := haveSet[id]; !ok {
			toAttach = append(toAttach, id)
		}
	}

	var toDetach []int
	for id := range haveSet {
		if _, ok := wantSet[id]; !ok {
			toDetach = append(toDetach, id)
		}
	}
	sort.Ints(toAttach)
	sort.Ints(toDetach)
	return toAttach, toDetach
}

func intSet(values []int) map[int]struct{} {
	out := make(map[int]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
