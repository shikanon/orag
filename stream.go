package orag

import "context"

// QueryEventType describes a typed query stream event.
type QueryEventType string

const (
	QueryEventResponse QueryEventType = "response"
	QueryEventDone     QueryEventType = "done"
	QueryEventError    QueryEventType = "error"
)

// QueryEvent is emitted by StreamQuery. Error is terminal; Done is emitted
// after a successful response.
type QueryEvent struct {
	Type     QueryEventType
	Response *QueryResponse
	Err      error
}

// StreamQuery exposes the SDK query lifecycle as typed events. The beta SDK
// emits one complete response followed by Done; it does not claim token-level
// generation streaming.
func (c *Client) StreamQuery(ctx context.Context, req QueryRequest) <-chan QueryEvent {
	events := make(chan QueryEvent, 2)
	go func() {
		defer close(events)
		if err := ctx.Err(); err != nil {
			events <- QueryEvent{Type: QueryEventError, Err: wrapError("stream_query", req.KnowledgeBaseID, req.TraceID, err)}
			return
		}
		response, err := c.Query(ctx, req)
		if err != nil {
			events <- QueryEvent{Type: QueryEventError, Err: err}
			return
		}
		if !sendQueryEvent(ctx, events, QueryEvent{Type: QueryEventResponse, Response: &response}, req) {
			return
		}
		sendQueryEvent(ctx, events, QueryEvent{Type: QueryEventDone}, req)
	}()
	return events
}

func sendQueryEvent(ctx context.Context, events chan<- QueryEvent, event QueryEvent, req QueryRequest) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		select {
		case events <- QueryEvent{Type: QueryEventError, Err: wrapError("stream_query", req.KnowledgeBaseID, req.TraceID, ctx.Err())}:
		default:
		}
		return false
	}
}
