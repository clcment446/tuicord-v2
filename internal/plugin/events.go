package plugin

import (
	"context"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Event names dispatched to plugins via tuicord.on. Payload shapes are
// documented on each constant; all snowflake fields arrive as decimal strings.
const (
	// EventReady fires once the gateway READY has populated the store. No payload.
	EventReady = "ready"
	// EventMessageCreate fires on an incoming message.
	// Payload: {id, channel_id, guild_id, author_id, author, content, bot}.
	EventMessageCreate = "message.create"
	// EventMessageUpdate fires when a message is edited. Payload: as create.
	EventMessageUpdate = "message.update"
	// EventMessageDelete fires when a message is removed.
	// Payload: {id, channel_id}.
	EventMessageDelete = "message.delete"
	// EventReactionAdd fires when a reaction is added.
	// Payload: {channel_id, message_id, user_id, emoji}.
	EventReactionAdd = "reaction.add"
	// EventReactionRemove fires when a reaction is removed. Payload: as add.
	EventReactionRemove = "reaction.remove"
	// EventChannelSwitch fires when the user selects a channel.
	// Payload: {guild_id, channel_id}.
	EventChannelSwitch = "channel.switch"
	// EventError fires when background client work fails. Payload: {message}.
	EventError = "error"
)

// subscription binds a Lua callback to the LState it belongs to. A callback
// must be invoked on its own state, so the state travels with the function.
type subscription struct {
	L      *lua.LState
	fn     *lua.LFunction
	plugin string
}

// eventBus is the fan-out registry backing tuicord.on. It is only ever touched
// on the plugin goroutine (registration during load, dispatch from event jobs),
// so it needs no locking.
type eventBus struct {
	subs map[string][]subscription
}

func newEventBus() *eventBus {
	return &eventBus{subs: make(map[string][]subscription)}
}

// on registers fn (owned by L) as a listener for event.
func (b *eventBus) on(event string, L *lua.LState, fn *lua.LFunction, plugin string) {
	b.subs[event] = append(b.subs[event], subscription{L: L, fn: fn, plugin: plugin})
}

// rollbackOwner discards registrations made before a plugin failed startup. It
// runs on the same worker as registration and dispatch.
func (b *eventBus) rollbackOwner(L *lua.LState) {
	for event, subs := range b.subs {
		kept := subs[:0]
		for _, sub := range subs {
			if sub.L != L {
				kept = append(kept, sub)
			}
		}
		for i := len(kept); i < len(subs); i++ {
			subs[i] = subscription{}
		}
		if len(kept) == 0 {
			delete(b.subs, event)
		} else {
			b.subs[event] = kept
		}
	}
}

// dispatch invokes every listener for event, building the payload table freshly
// for each subscriber's state (Lua values are state-specific). A callback error
// is reported via onErr and does not stop the remaining listeners.
func (b *eventBus) dispatch(ctx context.Context, timeout time.Duration, event string, payload map[string]any, onErr func(plugin string, err error)) {
	for _, sub := range b.subs[event] {
		arg := toLua(sub.L, mapAsAny(payload))
		if err := safeCall(ctx, sub.L, sub.fn, timeout, arg); err != nil && onErr != nil {
			onErr(sub.plugin, err)
		}
	}
}

// mapAsAny adapts a nil-safe payload map into the any form toLua consumes.
func mapAsAny(payload map[string]any) any {
	if payload == nil {
		return map[string]any{}
	}
	return payload
}
