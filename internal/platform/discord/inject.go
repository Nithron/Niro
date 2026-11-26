package discord

import "github.com/disgoorg/disgo/gateway"

// injectedShard stands in for a gateway connection when events are handed to
// the bot directly. disgo's handlers only ask it for the shard ID.
type injectedShard struct{ gateway.Gateway }

func (injectedShard) ShardID() int { return 0 }

// Inject hands a gateway event to the bot as if shard 0 had received it, and
// returns once the bot's synchronous handling is done. Only the load test
// (cmd/niro-soak) calls it; the running bot gets its events from Discord.
func (b *Bot) Inject(t gateway.EventType, seq int, data gateway.EventData) {
	b.client.EventManager.HandleGatewayEvent(injectedShard{}, t, seq, data)
}
