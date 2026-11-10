package pipeline

import (
	"fmt"
	"ladder/internal/connections"
	"ladder/internal/providers"
	"time"
)

type UsageCall struct {
	ID                 string          `json:"id"`
	Agent              string          `json:"agent"`
	At                 time.Time       `json:"at"`
	Usage              providers.Usage `json:"usage"`
	EstimatedUSD       *float64        `json:"estimated_usd"`
	InputRate          *float64        `json:"input_rate"`
	CachedInputRate    *float64        `json:"cached_input_rate"`
	OutputRate         *float64        `json:"output_rate"`
	CachedRateFallback bool            `json:"cached_rate_fallback"`
}

// Each response (including unusable paid output) is journaled before validation.
// Unknown usage stays unknown; timeouts can still be billable at the provider.
func (e *Engine) recordUsage(c *Candidate, agent string, u providers.Usage, cfg connections.AI) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	found := false
	for _, current := range e.candidates {
		if current.ID == c.ID {
			found = true
			if current.Live == nil || current.Live.Version != c.Live.Version {
				return ErrConflict
			}
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	e.mergeVoiceLocked(c)
	call := UsageCall{ID: fmt.Sprintf("%s-call-%d", c.ID, len(c.Live.Calls)+1), Agent: agent, At: time.Now().UTC(), Usage: u, InputRate: cfg.InputUSDPerMillion, CachedInputRate: cfg.CachedInputUSDPerMillion, OutputRate: cfg.OutputUSDPerMillion}
	if u.Billing != "subscription" && u.Reported && cfg.InputUSDPerMillion != nil && cfg.OutputUSDPerMillion != nil {
		cachedRate := *cfg.InputUSDPerMillion
		if cfg.CachedInputUSDPerMillion != nil {
			cachedRate = *cfg.CachedInputUSDPerMillion
		} else if u.CachedTokens > 0 {
			call.CachedRateFallback = true
		}
		estimate := (float64(u.InputTokens-u.CachedTokens)*(*cfg.InputUSDPerMillion) + float64(u.CachedTokens)*cachedRate + float64(u.OutputTokens)*(*cfg.OutputUSDPerMillion)) / 1e6
		call.EstimatedUSD = &estimate
	}
	c.Live.Calls = append(c.Live.Calls, call)
	return e.commit(record{Event: event(agent, "ai-usage", "AI attempt recorded. Token counts come from provider usage; prices are configured estimates."), Candidate: c})
}
