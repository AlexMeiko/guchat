package stream

type EventType string

const (
	EventTypeMessageCreated    EventType = "message.created"
	EventTypeMessageUpdated    EventType = "message.updated"
	EventTypeMessageDeleted    EventType = "message.deleted"
	EventTypeGenerationCompleted EventType = "generation.completed"
)
