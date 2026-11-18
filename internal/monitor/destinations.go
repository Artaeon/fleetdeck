package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"sort"
	"strconv"
	"strings"
)

func destinationHash(parts ...string) string {
	data, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func validDestination(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

// DestinationIdentity is an optional extension for custom providers. Unknown
// providers cannot own a durable backlog without an explicit destination ID.
func providerDestination(provider AlertProvider) (string, error) {
	if bound, ok := provider.(interface{ DestinationIdentity() string }); ok {
		identity := bound.DestinationIdentity()
		if !validDestination(identity) {
			return "", fmt.Errorf("notification destination requires a SHA-256 identity")
		}
		return identity, nil
	}
	switch p := provider.(type) {
	case *WebhookProvider:
		return destinationHash(p.Name(), p.URL), nil
	case *SlackProvider:
		return destinationHash(p.Name(), p.WebhookURL, p.Channel), nil
	case *DiscordProvider:
		return destinationHash(p.Name(), p.WebhookURL), nil
	case *TelegramProvider:
		// A token rotation for the same bot must not orphan pending messages.
		bot, _, _ := strings.Cut(p.BotToken, ":")
		return destinationHash(p.Name(), bot, strconv.FormatInt(p.ChatID, 10), strconv.FormatInt(p.ThreadID, 10)), nil
	case *EmailProvider:
		address := func(value string) string {
			if parsed, err := mail.ParseAddress(value); err == nil {
				return parsed.Address
			}
			return value
		}
		recipients := make([]string, len(p.To))
		for i, value := range p.To {
			recipients[i] = address(value)
		}
		sort.Strings(recipients)
		parts := []string{p.Name(), p.Host, strconv.Itoa(p.Port), p.Username, address(p.From)}
		return destinationHash(append(parts, recipients...)...), nil
	default:
		return "", fmt.Errorf("notification provider needs an explicit destination identity")
	}
}

func bindDestinations(state *alertDiskState, current map[string]string) error {
	for _, item := range state.Pending {
		for name, delivery := range item.Channels {
			if delivery.Delivered {
				continue
			}
			if identity, configured := current[name]; configured && state.Destinations[name] != identity {
				return fmt.Errorf("pending %s alerts have an unbound or changed destination; preserve the queue and finish delivery with the prior verified configuration/version", name)
			}
		}
	}
	if state.Destinations == nil {
		state.Destinations = map[string]string{}
	}
	for name, identity := range current {
		state.Destinations[name] = identity
	}
	return nil
}
