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

// Destinations records the original channel names and recipient fingerprints.
type Destinations map[string]string

// BindProviders snapshots the routing configuration at enqueue time.
func BindProviders(providers []AlertProvider) (Destinations, error) {

	destinations := Destinations{}
	if len(providers) > 32 {
		return nil, fmt.Errorf("at most 32 notification channels are supported")
	}
	for _, p := range providers {
		if !channelName.MatchString(p.Name()) || destinations[p.Name()] != "" {
			return nil, fmt.Errorf("alert providers need distinct bounded channel names")
		}

		identity, err := providerDestination(p)
		if err != nil {
			return nil, err
		}
		destinations[p.Name()] = identity
	}
	return destinations, nil
}

// Validate checks the persisted recipient set before a send or status read.
func (d Destinations) Validate() error {
	if len(d) == 0 || len(d) > 32 {
		return fmt.Errorf("invalid or unbound notification recipients")
	}
	for name, id := range d {
		if !channelName.MatchString(name) || !validDestination(id) {
			return fmt.Errorf("invalid notification recipient binding")
		}
	}
	return nil
}

// Check refuses a configured recipient change before any send.
// A channel missing from current must remain pending in the caller's checkpoint.
func (d Destinations) Check(current Destinations) error {
	if err := d.Validate(); err != nil {
		return err
	}
	for name, id := range d {
		if next, ok := current[name]; ok && id != next {
			return fmt.Errorf("pending %s destination changed; restore its verified routing configuration", name)
		}
	}
	return nil
}

// Verify checks the actual provider immediately before handing it a frozen message.
func (d Destinations) Verify(p AlertProvider) error {
	id, err := providerDestination(p)
	if err != nil || d[p.Name()] != id {
		return fmt.Errorf("pending notification recipient no longer matches its original destination")
	}
	return nil
}
