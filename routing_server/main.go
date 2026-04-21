// routing_server/main.go
//
// gRPC port: 9200  (configurable via -addr flag)

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/itsTurner/modss/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	listenAddr = flag.String("addr", ":9200", "Routing server listen address")
	routeTTL   = flag.Int("ttl", 60, "Route TTL in seconds before PoP must re-register")
	windowSize = flag.Int("window", 20, "Sliding window size for completion times per origin")
)


// Internal state types
type jobType struct {
	mlCensorship bool
	watermark    bool
	format       pb.Format
}

type completionRecord struct {
	durationMs int64
	at         time.Time
	region     string
}

type originState struct {
	mu            sync.RWMutex
	id            string
	address       string
	cpuUtil       float32
	activeStreams  int
	lastHeartbeat time.Time
	completions   []completionRecord
	chunks        map[int32]map[int32]bool // streamerID → chunkID → present
}

func (o *originState) addCompletion(r completionRecord) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.completions = append(o.completions, r)
	if len(o.completions) > *windowSize {
		o.completions = o.completions[1:]
	}
}

func (o *originState) avgCompletionMs(region string) float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
	var sum int64
	var n int
	for _, c := range o.completions {
		if region == "" || c.region == region {
			sum += c.durationMs
			n++
		}
	}
	if n == 0 {
		return 500 // neutral default
	}
	return float64(sum) / float64(n)
}

type streamRecord struct {
	config    *pb.StreamConfig
	origins   []*originState
	jobID     string
	createdAt time.Time
	popID     string
	popRegion string
}

type chunkRecord struct {
	originID string
}


// Routing server
type routingServer struct {
	pb.UnimplementedRoutingServer

	mu      sync.RWMutex
	origins map[string]*originState
	streams map[int32]*streamRecord
	chunks  map[int32]map[int32]*chunkRecord
}

func newRoutingServer() *routingServer {
	return &routingServer{
		origins: make(map[string]*originState),
		streams: make(map[int32]*streamRecord),
		chunks:  make(map[int32]map[int32]*chunkRecord),
	}
}


// Load-balancing / scoring
func (s *routingServer) scoreOrigin(o *originState, jt jobType, region string) float64 {
	o.mu.RLock()
	cpu := o.cpuUtil
	active := o.activeStreams
	o.mu.RUnlock()

	if cpu > 0.85 {
		return 1e9 // overloaded – avoid
	}

	avgMs := o.avgCompletionMs(region)
	score := float64(cpu)*100 + float64(active)*2 + avgMs/10

	// Job-similarity bonus: prefer origins already running the same pipeline
	s.mu.RLock()
	for _, sr := range s.streams {
		if len(sr.origins) > 0 && sr.origins[0].id == o.id {
			srJT := jobType{sr.config.EnableMlCensorship, sr.config.EnableWatermark, sr.config.VideoFormat}
			if srJT == jt {
				score -= 5
			}
		}
	}
	s.mu.RUnlock()

	return score
}

func (s *routingServer) pickOrigins(jt jobType, region string, n int) []*originState {
	s.mu.RLock()
	candidates := make([]*originState, 0, len(s.origins))
	for _, o := range s.origins {
		o.mu.RLock()
		alive := time.Since(o.lastHeartbeat) < 30*time.Second
		o.mu.RUnlock()
		if alive {
			candidates = append(candidates, o)
		}
	}
	s.mu.RUnlock()

	if len(candidates) == 0 {
		return nil
	}

	type scored struct {
		o     *originState
		score float64
	}
	ss := make([]scored, len(candidates))
	for i, o := range candidates {
		ss[i] = scored{o, s.scoreOrigin(o, jt, region) + rand.Float64()*2}
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].score < ss[j].score })

	if n > len(ss) {
		n = len(ss)
	}
	result := make([]*originState, n)
	for i := range result {
		result[i] = ss[i].o
	}
	return result
}


// gRPC handlers
func (s *routingServer) RegisterStream(ctx context.Context, req *pb.RegisterStreamRequest) (*pb.RegisterStreamResponse, error) {
	return s.doRegister(req.Config, req.PopId, req.PopRegion)
}

func (s *routingServer) RefreshRoute(ctx context.Context, req *pb.RefreshRouteRequest) (*pb.RegisterStreamResponse, error) {
	s.mu.RLock()
	rec, ok := s.streams[req.StreamerId]
	s.mu.RUnlock()
	if !ok {
		return &pb.RegisterStreamResponse{Success: false, Error: "stream not registered"}, nil
	}
	log.Printf("[routing] refresh route stream=%d reason=%s", req.StreamerId, req.Reason)
	return s.doRegister(rec.config, rec.popID, rec.popRegion)
}

func (s *routingServer) doRegister(cfg *pb.StreamConfig, popID, popRegion string) (*pb.RegisterStreamResponse, error) {
	if cfg == nil {
		return &pb.RegisterStreamResponse{Success: false, Error: "nil config"}, nil
	}
	jt := jobType{cfg.EnableMlCensorship, cfg.EnableWatermark, cfg.VideoFormat}
	origins := s.pickOrigins(jt, popRegion, 2)
	if len(origins) == 0 {
		return &pb.RegisterStreamResponse{Success: false, Error: "no healthy origins available"}, nil
	}

	refs := make([]*pb.OriginRef, len(origins))
	for i, o := range origins {
		refs[i] = &pb.OriginRef{OriginId: o.id, Address: o.address}
	}

	s.mu.Lock()
	s.streams[cfg.StreamerId] = &streamRecord{
		config:    cfg,
		origins:   origins,
		jobID:     uuid.NewString(),
		createdAt: time.Now(),
		popID:     popID,
		popRegion: popRegion,
	}
	s.mu.Unlock()

	log.Printf("[routing] stream %d assigned → %v (pop=%s region=%s)", cfg.StreamerId, refs, popID, popRegion)
	return &pb.RegisterStreamResponse{
		Success:    true,
		Origins:    refs,
		TtlSeconds: int32(*routeTTL),
	}, nil
}

func (s *routingServer) JobComplete(ctx context.Context, req *pb.JobCompleteRequest) (*pb.JobCompleteResponse, error) {
	s.mu.RLock()
	o := s.origins[req.OriginId]
	rec := s.streams[req.StreamerId]
	s.mu.RUnlock()

	region := ""
	if rec != nil {
		region = rec.popRegion
	}
	if o != nil {
		o.addCompletion(completionRecord{durationMs: req.DurationMs, at: time.Now(), region: region})
	}

	if req.ChunkId >= 0 && req.OriginId != "" {
		s.mu.Lock()
		if s.chunks[req.StreamerId] == nil {
			s.chunks[req.StreamerId] = make(map[int32]*chunkRecord)
		}
		s.chunks[req.StreamerId][req.ChunkId] = &chunkRecord{originID: req.OriginId}
		s.mu.Unlock()

		if o != nil {
			o.mu.Lock()
			if o.chunks[req.StreamerId] == nil {
				o.chunks[req.StreamerId] = make(map[int32]bool)
			}
			o.chunks[req.StreamerId][req.ChunkId] = true
			o.mu.Unlock()
		}
	}

	log.Printf("[routing] job done origin=%s stream=%d chunk=%d %dms ok=%v",
		req.OriginId, req.StreamerId, req.ChunkId, req.DurationMs, req.Success)
	return &pb.JobCompleteResponse{Success: true}, nil
}

func (s *routingServer) GetChunkLocation(ctx context.Context, req *pb.ChunkLocationRequest) (*pb.ChunkLocationResponse, error) {
	s.mu.RLock()
	var cr *chunkRecord
	if m := s.chunks[req.StreamerId]; m != nil {
		cr = m[req.ChunkId]
	}
	var o *originState
	if cr != nil {
		o = s.origins[cr.originID]
	}
	s.mu.RUnlock()

	if cr == nil {
		return &pb.ChunkLocationResponse{
			Success: false,
			Error:   fmt.Sprintf("chunk %d for stream %d not found", req.ChunkId, req.StreamerId),
		}, nil
	}
	if o == nil {
		return &pb.ChunkLocationResponse{Success: false, Error: "origin no longer available"}, nil
	}
	return &pb.ChunkLocationResponse{
		Success: true,
		Origin:  &pb.OriginRef{OriginId: o.id, Address: o.address},
	}, nil
}

func (s *routingServer) OriginHeartbeat(ctx context.Context, req *pb.OriginHeartbeatRequest) (*pb.OriginHeartbeatResponse, error) {
	s.mu.Lock()
	o, exists := s.origins[req.OriginId]
	if !exists {
		o = &originState{id: req.OriginId, address: req.Address, chunks: make(map[int32]map[int32]bool)}
		s.origins[req.OriginId] = o
		log.Printf("[routing] new origin: id=%s addr=%s", req.OriginId, req.Address)
	}
	s.mu.Unlock()

	o.mu.Lock()
	o.cpuUtil = req.CpuUtilization
	o.activeStreams = int(req.ActiveStreams)
	o.lastHeartbeat = time.Now()
	if req.Address != "" {
		o.address = req.Address
	}
	o.mu.Unlock()

	return &pb.OriginHeartbeatResponse{Success: true}, nil
}


// Main
func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.MaxSendMsgSize(16*1024*1024),
	)
	rs := newRoutingServer()
	pb.RegisterRoutingServer(srv, rs)
	reflection.Register(srv)

	log.Printf("MODSS Routing Server listening on %s", *listenAddr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve error: %v", err)
	}
}