// Package middleware shows how to wire DeepMemory recall + symbolism
// extraction into an eino v0.9 gateway WITHOUT relying on dialog hooks.
//
// This file is illustrative — copy into your gateway repo and adapt
// imports to your project layout. See docs/integration/gateway_integration_eino.md
// for the architectural rationale.
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config bundles both middlewares' settings. Hydrate from fm.yaml.
type Config struct {
	Augmentation MemoryAugmentationConfig
	Symbolism    SymbolismAsyncConfig
}

// MemoryAugmentationConfig — entry middleware: recall + inject.
type MemoryAugmentationConfig struct {
	Client          DeepMemoryClient
	Enabled         bool
	MaxRecall       int
	MinConfidence   float64
	SessionIDKey    string // ctx key, e.g. "session_id"
	UserIDKey       string // ctx key, e.g. "user_id"
	InjectAsSystem  bool   // true = standalone system msg; false = append to existing
	RecallTimeoutMs int    // hard cap; 0 = no timeout
}

// SymbolismAsyncConfig — exit middleware: async ingest.
type SymbolismAsyncConfig struct {
	Client       DeepMemoryClient
	Enabled      bool
	SessionIDKey string
	UserIDKey    string
	IngestTimeoutSec int // default 30
}

// ---------------------------------------------------------------------------
// DeepMemoryClient — minimal interface the middlewares need.
//
// Implement this against MCP stdio JSON-RPC, eino-ext MCP binding, or HTTP —
// whatever your stack prefers.
// ---------------------------------------------------------------------------

type DeepMemoryClient interface {
	RecallMulti(ctx context.Context, req RecallMultiReq) ([]Record, error)
	IngestAsync(ctx context.Context, req IngestAsyncReq) (taskID string, err error)
}

type RecallMultiReq struct {
	ObjectIDs     []string `json:"object_ids"`
	Limit         int      `json:"limit"`
	MinConfidence float64  `json:"min_confidence"`
}

type IngestAsyncReq struct {
	TurnText  string `json:"turn_text"`
	SessionID string `json:"session_id"`
	ActorRef  string `json:"actor_ref"`
	SourceRef string `json:"source_ref"`
}

type Record struct {
	MemoryID         string  `json:"memory_id"`
	CanonicalSign    string  `json:"canonical_sign"`
	OperationalClaim string  `json:"operational_claim"`
	SocialStatus     string  `json:"social_status"`
	Confidence       float64 `json:"confidence"`
	SourceRef        string  `json:"source_ref"`
}

// ---------------------------------------------------------------------------
// Entry middleware: AugmentMessages
// ---------------------------------------------------------------------------

// AugmentMessages prepends recalled DeepMemory observations to the messages
// slice as a system context block. It NEVER blocks the conversation:
// errors and timeouts result in passing through the original messages.
func (cfg *MemoryAugmentationConfig) AugmentMessages(
	ctx context.Context,
	messages []*schema.Message,
) ([]*schema.Message, error) {
	if !cfg.Enabled || cfg.Client == nil {
		return messages, nil
	}

	sessionID, _ := ctx.Value(cfg.SessionIDKey).(string)
	userID, _ := ctx.Value(cfg.UserIDKey).(string)

	objectIDs := make([]string, 0, 2)
	if sessionID != "" {
		objectIDs = append(objectIDs, "session:"+sessionID)
	}
	if userID != "" {
		objectIDs = append(objectIDs, "user:"+userID)
	}
	if len(objectIDs) == 0 {
		return messages, nil
	}

	limit := cfg.MaxRecall
	if limit <= 0 {
		limit = 10
	}
	minConf := cfg.MinConfidence
	if minConf <= 0 {
		minConf = 0.35
	}

	callCtx := ctx
	if cfg.RecallTimeoutMs > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.RecallTimeoutMs)*time.Millisecond)
		defer cancel()
	}

	memories, err := cfg.Client.RecallMulti(callCtx, RecallMultiReq{
		ObjectIDs:     objectIDs,
		Limit:         limit,
		MinConfidence: minConf,
	})
	if err != nil || len(memories) == 0 {
		// Recall failure must never block conversation.
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[memory-augmentation] recall failed: %v", err)
		}
		return messages, nil
	}

	block := buildMemoryContextBlock(memories)

	if cfg.InjectAsSystem {
		return append([]*schema.Message{schema.SystemMessage(block)}, messages...), nil
	}

	for i, m := range messages {
		if m.Role == schema.System {
			messages[i] = schema.SystemMessage(m.Content + "\n\n" + block)
			return messages, nil
		}
	}
	return append([]*schema.Message{schema.SystemMessage(block)}, messages...), nil
}

// buildMemoryContextBlock renders recalled records as a markdown-flavoured
// system context block. Keep it short — every token here counts.
func buildMemoryContextBlock(memories []Record) string {
	var sb strings.Builder
	sb.WriteString("## Recalled Memories (DeepMemory)\n\n")
	sb.WriteString("Persistent observations from prior sessions. ")
	sb.WriteString("CANONICAL = treat as fact; PROVISIONAL_CONSENSUS = reliable; ")
	sb.WriteString("LOCAL_HYPOTHESIS = single-source; CONTESTED = ignore.\n\n")
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf(
			"- **[%s]** (%s, conf=%.2f) %s — _src: %s_\n",
			m.CanonicalSign,
			strings.ToUpper(m.SocialStatus),
			m.Confidence,
			m.OperationalClaim,
			m.SourceRef,
		))
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Exit middleware: FireAndForget
// ---------------------------------------------------------------------------

// FireAndForget pushes the user→assistant turn into DeepMemory's async
// extraction queue. Always returns immediately; the heavy lifting happens
// on a detached background context.
//
// IMPORTANT: do NOT propagate the request context — once the HTTP handler
// returns, that context is cancelled and your async task gets killed mid-flight.
func (cfg *SymbolismAsyncConfig) FireAndForget(
	ctx context.Context,
	userMessage string,
	assistantReply *schema.Message,
) {
	if !cfg.Enabled || cfg.Client == nil || assistantReply == nil {
		return
	}
	sessionID, _ := ctx.Value(cfg.SessionIDKey).(string)
	userID, _ := ctx.Value(cfg.UserIDKey).(string)

	timeout := cfg.IngestTimeoutSec
	if timeout <= 0 {
		timeout = 30
	}

	payload := IngestAsyncReq{
		TurnText:  fmt.Sprintf("USER: %s\n\nASSISTANT: %s", userMessage, assistantReply.Content),
		SessionID: sessionID,
		ActorRef:  "user:" + userID,
		SourceRef: fmt.Sprintf("session-%s-turn-%d", sessionID, time.Now().UnixNano()),
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		if _, err := cfg.Client.IngestAsync(bgCtx, payload); err != nil {
			log.Printf("[symbolism-async] ingest failed: %v", err)
		}
	}()
}

// ---------------------------------------------------------------------------
// Putting it together — example HTTP handler
// ---------------------------------------------------------------------------
//
// Outside this file you'd construct your *react.Agent and pass it in, e.g.:
//
//   func main() {
//       cfg := loadFromYAML("fm.yaml")
//       client := deepmemory.NewStdioClient(...)
//
//       memMW := middleware.MemoryAugmentationConfig{
//           Client: client, Enabled: true, MaxRecall: 10, MinConfidence: 0.35,
//           SessionIDKey: "session_id", UserIDKey: "user_id",
//           InjectAsSystem: false, RecallTimeoutMs: 200,
//       }
//       symMW := middleware.SymbolismAsyncConfig{
//           Client: client, Enabled: true,
//           SessionIDKey: "session_id", UserIDKey: "user_id",
//           IngestTimeoutSec: 30,
//       }
//
//       agent := buildEinoReactAgent(cfg)
//       http.HandleFunc("/api/chat", middleware.ChatHandler(agent, &memMW, &symMW))
//       http.ListenAndServe(":8080", nil)
//   }
//

// ChatRequest mirrors what your gateway already accepts.
type ChatRequest struct {
	SessionID    string `json:"session_id"`
	UserID       string `json:"user_id"`
	SystemPrompt string `json:"system_prompt"`
	UserMessage  string `json:"user_message"`
}

// ChatResponse is whatever your existing API contract uses — stub shown.
type ChatResponse struct {
	Reply string `json:"reply"`
}

// EinoAgent is the minimal contract this example needs from your react.Agent.
// In production, replace with the actual *react.Agent type.
type EinoAgent interface {
	Generate(ctx context.Context, msgs []*schema.Message) (*schema.Message, error)
}

// ChatHandler wires the two middlewares around the agent call.
func ChatHandler(
	agent EinoAgent,
	memMW *MemoryAugmentationConfig,
	symMW *SymbolismAsyncConfig,
) func(w writer, r reader) {
	// NOTE: w/r are net/http types in real code. This signature is a stub
	// to keep the example self-contained without pulling net/http here.
	return func(w writer, r reader) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body()).Decode(&req); err != nil {
			w.WriteHeader(400)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, memMW.SessionIDKey, req.SessionID)
		ctx = context.WithValue(ctx, memMW.UserIDKey, req.UserID)

		msgs := []*schema.Message{
			schema.SystemMessage(req.SystemPrompt),
			schema.UserMessage(req.UserMessage),
		}

		// ★ Entry middleware
		msgs, _ = memMW.AugmentMessages(ctx, msgs)

		reply, err := agent.Generate(ctx, msgs)
		if err != nil {
			w.WriteHeader(500)
			return
		}

		// ★ Exit middleware (non-blocking)
		symMW.FireAndForget(ctx, req.UserMessage, reply)

		_ = json.NewEncoder(w).Encode(ChatResponse{Reply: reply.Content})
	}
}

// ---------------------------------------------------------------------------
// Stub I/O types so this file compiles without pulling net/http.
// In your real gateway, replace these with http.ResponseWriter / *http.Request.
// ---------------------------------------------------------------------------

type writer interface {
	WriteHeader(int)
	Write([]byte) (int, error)
}

type reader interface {
	Body() interface {
		Read([]byte) (int, error)
	}
	Context() context.Context
}
