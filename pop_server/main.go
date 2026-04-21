// pop_server/main.go
//
// Ports:
//   1935  RTMP ingest
//   8080  HTTP: WebRTC signalling (/whip), HLS viewer (/live/<id>/playlist.m3u8,
//               /live/<id>/<chunk>.ts), health (/healthz)
//   Configurable via flags.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nareix/joy4/av/avutil"
	"github.com/nareix/joy4/format/flv"
	"github.com/nareix/joy4/format/rtmp"
	"github.com/pion/webrtc/v3"
	pb "github.com/itsTurner/modss/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Flags
var (
	rtmpAddr    = flag.String("rtmp", ":1935", "RTMP listen address")
	httpAddr    = flag.String("http", ":8080", "HTTP listen address (WebRTC signalling + HLS)")
	routingAddr = flag.String("routing", "localhost:9200", "Routing server address")
	popID       = flag.String("id", "pop-1", "Unique PoP identifier")
	popRegion   = flag.String("region", "us-west", "Geographic region label")

	// Default transcode parameters (can be overridden per-stream in production)
	defaultVideoRes      = flag.String("res", "p1080", "Default output resolution")
	defaultFrameRate     = flag.String("fps", "fps30", "Default frame rate")
	defaultVideoBitrate  = flag.Int("vbr", 4, "Default video bitrate Mbps")
	defaultAudioBitrate  = flag.Int("abr", 128, "Default audio bitrate kbps")
)

// Stream state
type streamInfo struct {
	mu           sync.Mutex
	streamerID   int32
	streamKey    string
	config       *pb.StreamConfig
	origins      []*pb.OriginRef
	routeTTL     time.Time
	chunkCounter int32         // last chunk sent to origin
	active       atomic.Bool
}

// PoP server
type popServer struct {
	mu sync.RWMutex

	popID     string
	popRegion string

	routingConn   *grpc.ClientConn
	routingClient pb.RoutingClient

	// active stream keyed by stream key string (RTMP app/stream key)
	streams map[string]*streamInfo
	// same by integer ID
	streamsByID map[int32]*streamInfo

	// monotonic stream-id counter
	nextID atomic.Int32

	// gRPC connections to origins (lazily opened, keyed by origin address)
	originConns   map[string]*grpc.ClientConn
	originConnsMu sync.Mutex
}

func newPopServer() (*popServer, error) {
	conn, err := grpc.Dial(*routingAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(16*1024*1024),
			grpc.MaxCallSendMsgSize(16*1024*1024),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("routing dial: %w", err)
	}
	return &popServer{
		popID:         *popID,
		popRegion:     *popRegion,
		routingConn:   conn,
		routingClient: pb.NewRoutingClient(conn),
		streams:       make(map[string]*streamInfo),
		streamsByID:   make(map[int32]*streamInfo),
		originConns:   make(map[string]*grpc.ClientConn),
	}, nil
}

// Origin gRPC helpers
func (p *popServer) originClient(addr string) (pb.OriginClient, error) {
	p.originConnsMu.Lock()
	defer p.originConnsMu.Unlock()
	conn, ok := p.originConns[addr]
	if !ok {
		var err error
		conn, err = grpc.Dial(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(16*1024*1024),
				grpc.MaxCallSendMsgSize(16*1024*1024),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("origin dial %s: %w", addr, err)
		}
		p.originConns[addr] = conn
	}
	return pb.NewOriginClient(conn), nil
}

// originForStream returns the preferred (first) origin for a stream.
func (p *popServer) originForStream(si *streamInfo) (*pb.OriginRef, error) {
	si.mu.Lock()
	defer si.mu.Unlock()
	if len(si.origins) == 0 {
		return nil, fmt.Errorf("no origins assigned for stream %d", si.streamerID)
	}
	// Re-register if TTL has expired
	if time.Now().After(si.routeTTL) {
		resp, err := p.routingClient.RefreshRoute(context.Background(), &pb.RefreshRouteRequest{
			StreamerId: si.streamerID,
			PopId:      p.popID,
			Reason:     "ttl_expired",
		})
		if err == nil && resp.Success {
			si.origins = resp.Origins
			si.routeTTL = time.Now().Add(time.Duration(resp.TtlSeconds) * time.Second)
		}
	}
	return si.origins[0], nil
}

// sendChunkToOrigin sends a single video chunk via gRPC streaming.
func (p *popServer) sendChunkToOrigin(si *streamInfo, data []byte) error {
	ref, err := p.originForStream(si)
	if err != nil {
		return err
	}
	client, err := p.originClient(ref.Address)
	if err != nil {
		return err
	}

	si.mu.Lock()
	chunkID := atomic.AddInt32(&si.chunkCounter, 1)
	jobID := uuid.NewString()
	cfg := si.config
	si.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := client.IngestVideoRpc(ctx)
	if err != nil {
		return fmt.Errorf("ingest_video_rpc: %w", err)
	}

	start := time.Now()
	if err := stream.Send(&pb.IngestVideoRequest{
		StreamerId:         cfg.StreamerId,
		VideoFormat:        cfg.VideoFormat,
		VideoCodec:         cfg.VideoCodec,
		AudioCodec:         cfg.AudioCodec,
		VideoData:          data,
		EnableMlCensorship: cfg.EnableMlCensorship,
		EnableWatermark:    cfg.EnableWatermark,
		VideoRes:           cfg.VideoRes,
		FrameRate:          cfg.FrameRate,
		VideoBitrateMbps:   cfg.VideoBitrateMbps,
		AudioBitrateKbps:   cfg.AudioBitrateKbps,
		JobId:              jobID,
	}); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		if !resp.Success {
			log.Printf("[pop] origin error stream=%d chunk=%d: %s", si.streamerID, chunkID, resp.Error)
		}
	}

	// Report completion to routing server (best-effort)
	durMs := time.Since(start).Milliseconds()
	go func() {
		_, _ = p.routingClient.JobComplete(context.Background(), &pb.JobCompleteRequest{
			OriginId:   ref.OriginId,
			JobId:      jobID,
			StreamerId: si.streamerID,
			ChunkId:    chunkID,
			Success:    true,
			DurationMs: durMs,
		})
	}()

	return nil
}

// fetchChunkFromOrigin retrieves a chunk from the appropriate origin.
func (p *popServer) fetchChunkFromOrigin(streamerID, chunkID int32) ([]byte, error) {
	// Ask routing server which origin has this chunk
	locResp, err := p.routingClient.GetChunkLocation(context.Background(), &pb.ChunkLocationRequest{
		StreamerId: streamerID,
		ChunkId:    chunkID,
		PopId:      p.popID,
	})
	if err != nil || !locResp.Success {
		errStr := "unknown"
		if locResp != nil {
			errStr = locResp.Error
		}
		return nil, fmt.Errorf("chunk location: %s", errStr)
	}

	client, err := p.originClient(locResp.Origin.Address)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.FetchChunkRpc(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&pb.FetchChunkRequest{StreamerId: streamerID, ChunkId: chunkID}); err != nil {
		return nil, err
	}
	_ = stream.CloseSend()

	var chunks [][]byte
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !resp.Success {
			return nil, fmt.Errorf("fetch_chunk error: %s", resp.Error)
		}
		chunks = append(chunks, resp.ChunkData)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("empty response for chunk %d", chunkID)
	}
	return chunks[0], nil
}

// Stream registration
func resEnum(s string) pb.Resolution {
	switch s {
	case "p720":
		return pb.Resolution_p720
	case "p480":
		return pb.Resolution_p480
	case "p360":
		return pb.Resolution_p360
	case "p240":
		return pb.Resolution_p240
	default:
		return pb.Resolution_p1080
	}
}

func fpsEnum(s string) pb.FrameRate {
	if s == "fps60" {
		return pb.FrameRate_fps60
	}
	return pb.FrameRate_fps30
}

func (p *popServer) registerStream(streamKey string) (*streamInfo, error) {
	id := p.nextID.Add(1)

	cfg := &pb.StreamConfig{
		StreamerId:       id,
		StreamKey:        streamKey,
		VideoFormat:      pb.Format_HLS,
		VideoCodec:       pb.VideoCodec_H264,
		AudioCodec:       pb.AudioCodec_AAC,
		VideoRes:         resEnum(*defaultVideoRes),
		FrameRate:        fpsEnum(*defaultFrameRate),
		VideoBitrateMbps: int32(*defaultVideoBitrate),
		AudioBitrateKbps: int32(*defaultAudioBitrate),
	}

	resp, err := p.routingClient.RegisterStream(context.Background(), &pb.RegisterStreamRequest{
		Config:    cfg,
		PopId:     p.popID,
		PopRegion: p.popRegion,
	})
	if err != nil {
		return nil, fmt.Errorf("routing register: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("routing denied: %s", resp.Error)
	}

	si := &streamInfo{
		streamerID: id,
		streamKey:  streamKey,
		config:     cfg,
		origins:    resp.Origins,
		routeTTL:   time.Now().Add(time.Duration(resp.TtlSeconds) * time.Second),
	}
	si.active.Store(true)

	p.mu.Lock()
	p.streams[streamKey] = si
	p.streamsByID[id] = si
	p.mu.Unlock()

	log.Printf("[pop] stream registered key=%s id=%d → %v", streamKey, id, resp.Origins)
	return si, nil
}

func (p *popServer) deregisterStream(streamKey string) {
	p.mu.Lock()
	si, ok := p.streams[streamKey]
	if ok {
		si.active.Store(false)
		delete(p.streams, streamKey)
		delete(p.streamsByID, si.streamerID)
	}
	p.mu.Unlock()
	if ok {
		log.Printf("[pop] stream ended key=%s id=%d", streamKey, si.streamerID)
	}
}

func (p *popServer) handleRTMP() {
	srv := &rtmp.Server{}
	srv.HandlePublish = func(conn *rtmp.Conn) {
		streams, err := conn.Streams()
		if err != nil {
			log.Printf("[rtmp] streams err: %v", err)
			return
		}
		// Use URL path as stream key: rtmp://host/live/<streamKey>
		streamKey := conn.URL.Path
		streamKey = strings.TrimPrefix(streamKey, "/")

		log.Printf("[rtmp] publish start key=%s", streamKey)
		si, err := p.registerStream(streamKey)
		if err != nil {
			log.Printf("[rtmp] register failed key=%s: %v", streamKey, err)
			return
		}
		defer p.deregisterStream(streamKey)

		// Demux RTMP → raw FLV bytes, then forward to origin in ~5 s batches.
		// We use joy4's avutil.CopyFile to a custom flv muxer writing to a
		// pipe so we can read the bytes and forward them.
		pr, pw := io.Pipe()

		mux := flv.NewMuxer(pw)
		if err := mux.WriteHeader(streams); err != nil {
			log.Printf("[rtmp] mux header: %v", err)
			return
		}

		// Read from pipe in background and forward chunks
		go func() {
			defer pr.Close()
			buf := make([]byte, 0, 4*1024*1024)
			tmp := make([]byte, 64*1024)
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			flush := func() {
				if len(buf) == 0 {
					return
				}
				chunk := make([]byte, len(buf))
				copy(chunk, buf)
				buf = buf[:0]
				if err := p.sendChunkToOrigin(si, chunk); err != nil {
					log.Printf("[rtmp] send chunk: %v", err)
				}
			}

			for {
				select {
				case <-ticker.C:
					flush()
				default:
					n, err := pr.Read(tmp)
					if n > 0 {
						buf = append(buf, tmp[:n]...)
					}
					if err != nil {
						flush()
						return
					}
				}
			}
		}()

		// Copy all packets from RTMP connection into the FLV muxer
		if err := avutil.CopyFile(mux, conn); err != nil && err != io.EOF {
			log.Printf("[rtmp] copy err key=%s: %v", streamKey, err)
		}
		pw.Close()
		log.Printf("[rtmp] publish end key=%s", streamKey)
	}

	log.Printf("[rtmp] listening on %s", *rtmpAddr)
	lis, err := net.Listen("tcp", *rtmpAddr)
	if err != nil {
		log.Fatalf("rtmp listen: %v", err)
	}
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("rtmp serve: %v", err)
	}
}

// ---------------------------------------------------------------------------
// WebRTC ingest (WHIP – WebRTC-HTTP Ingest Protocol)
// ---------------------------------------------------------------------------
// Browsers/OBS send an HTTP POST to /whip/<streamKey> with an SDP offer.
// We respond with an SDP answer and start forwarding RTP to the origin.

func (p *popServer) handleWHIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	streamKey := strings.TrimPrefix(r.URL.Path, "/whip/")
	if streamKey == "" {
		http.Error(w, "missing stream key", http.StatusBadRequest)
		return
	}

	offerSDP, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	si, err := p.registerStream(streamKey)
	if err != nil {
		http.Error(w, "register: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Build a Pion PeerConnection
	api := webrtc.NewAPI()
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		p.deregisterStream(streamKey)
		http.Error(w, "pc: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Accept video and audio
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		log.Printf("[whip] add video transceiver: %v", err)
	}
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		log.Printf("[whip] add audio transceiver: %v", err)
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[whip] track key=%s kind=%s codec=%s", streamKey, track.Kind(), track.Codec().MimeType)

		// Accumulate ~5 s of RTP payload bytes, then forward to origin.
		var buf []byte
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		rtpBuf := make([]byte, 1400)
		for {
			select {
			case <-ticker.C:
				if len(buf) > 0 {
					chunk := make([]byte, len(buf))
					copy(chunk, buf)
					buf = buf[:0]
					if err := p.sendChunkToOrigin(si, chunk); err != nil {
						log.Printf("[whip] send chunk: %v", err)
					}
				}
			default:
				n, _, err := track.Read(rtpBuf)
				if err != nil {
					return
				}
				buf = append(buf, rtpBuf[:n]...)
			}
		}
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("[whip] key=%s state=%s", streamKey, s)
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			p.deregisterStream(streamKey)
			_ = pc.Close()
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: string(offerSDP)}); err != nil {
		p.deregisterStream(streamKey)
		http.Error(w, "set remote: "+err.Error(), http.StatusBadRequest)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		p.deregisterStream(streamKey)
		http.Error(w, "answer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	gatherDone := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		p.deregisterStream(streamKey)
		http.Error(w, "local desc: "+err.Error(), http.StatusInternalServerError)
		return
	}
	<-gatherDone

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/whip/"+streamKey)
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprint(w, pc.LocalDescription().SDP)
}

// ---------------------------------------------------------------------------
// HLS viewer delivery
// ---------------------------------------------------------------------------
//
// GET /live/<streamerID>/playlist.m3u8  → dynamic playlist
// GET /live/<streamerID>/<chunkID>.ts   → fetch chunk from origin

func (p *popServer) handleHLS(w http.ResponseWriter, r *http.Request) {
	// Parse path: /live/<streamerID>/...
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/live/"), "/")
	if len(parts) < 2 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	idInt, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	streamerID := int32(idInt)

	p.mu.RLock()
	si := p.streamsByID[streamerID]
	p.mu.RUnlock()

	if si == nil || !si.active.Load() {
		http.Error(w, "stream not found", http.StatusNotFound)
		return
	}

	if parts[1] == "playlist.m3u8" {
		// Build a live HLS playlist with the last few chunks.
		lastChunk := atomic.LoadInt32(&si.chunkCounter)
		var sb strings.Builder
		sb.WriteString("#EXTM3U\n")
		sb.WriteString("#EXT-X-VERSION:3\n")
		sb.WriteString("#EXT-X-TARGETDURATION:5\n")
		sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", max32(1, lastChunk-2)))
		for i := max32(1, lastChunk-2); i <= lastChunk; i++ {
			sb.WriteString("#EXTINF:5.0,\n")
			sb.WriteString(fmt.Sprintf("/live/%d/%d.ts\n", streamerID, i))
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = io.WriteString(w, sb.String())
		return
	}

	// Chunk request: <chunkID>.ts
	seg := strings.TrimSuffix(parts[1], ".ts")
	chunkID, err := strconv.Atoi(seg)
	if err != nil {
		http.Error(w, "bad chunk", http.StatusBadRequest)
		return
	}

	data, err := p.fetchChunkFromOrigin(streamerID, int32(chunkID))
	if err != nil {
		log.Printf("[hls] fetch err stream=%d chunk=%d: %v", streamerID, chunkID, err)
		http.Error(w, "chunk not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "max-age=30")
	_, _ = w.Write(data)
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func (p *popServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	n := len(p.streams)
	p.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"pop_id":         p.popID,
		"pop_region":     p.popRegion,
		"active_streams": n,
	})
}

func (p *popServer) startHTTP() {
	mux := http.NewServeMux()
	mux.HandleFunc("/whip/", p.handleWHIP)
	mux.HandleFunc("/live/", p.handleHLS)
	mux.HandleFunc("/healthz", p.handleHealth)

	log.Printf("[http] listening on %s", *httpAddr)
	if err := http.ListenAndServe(*httpAddr, mux); err != nil {
		log.Fatalf("http serve: %v", err)
	}
}

func main() {
	flag.Parse()

	p, err := newPopServer()
	if err != nil {
		log.Fatalf("init pop server: %v", err)
	}

	// RTMP server in a goroutine
	go p.handleRTMP()

	// HTTP (WebRTC + HLS) in a goroutine
	go p.startHTTP()

	log.Printf("MODSS PoP Server started (id=%s region=%s)", p.popID, p.popRegion)

	// Block forever
	select {}
}