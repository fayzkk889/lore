package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/display"
)

type modelInfo struct {
	ID          string
	Name        string
	Provider    string
	Source      string
	Recommended bool
}

var curatedModels = map[string][]modelInfo{
	"anthropic": {
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet", Recommended: true},
		{ID: "claude-opus-4-1", Name: "Claude Opus", Recommended: true},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku"},
	},
	"openai": {
		{ID: "gpt-5", Name: "GPT-5", Recommended: true},
		{ID: "gpt-5-mini", Name: "GPT-5 mini", Recommended: true},
		{ID: "gpt-4.1", Name: "GPT-4.1"},
		{ID: "gpt-4.1-mini", Name: "GPT-4.1 mini"},
		{ID: "gpt-4o", Name: "GPT-4o"},
	},
	"openrouter": {
		{ID: "anthropic/claude-sonnet-4.5", Name: "Claude Sonnet", Recommended: true},
		{ID: "anthropic/claude-opus-4.1", Name: "Claude Opus", Recommended: true},
		{ID: "openai/gpt-5", Name: "GPT-5", Recommended: true},
		{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", Recommended: true},
		{ID: "deepseek/deepseek-r1", Name: "DeepSeek R1"},
		{ID: "deepseek/deepseek-chat-v3.1", Name: "DeepSeek Chat V3.1"},
		{ID: "moonshotai/kimi-k2.7-code", Name: "Kimi K2.7 Code"},
	},
	"deepseek": {
		{ID: "deepseek-chat", Name: "DeepSeek Chat", Recommended: true},
		{ID: "deepseek-reasoner", Name: "DeepSeek Reasoner", Recommended: true},
	},
	"ollama": {
		{ID: "qwen3:4b", Name: "Qwen 3 4B", Recommended: true},
		{ID: "llama3.1:8b", Name: "Llama 3.1 8B"},
		{ID: "deepseek-r1:8b", Name: "DeepSeek R1 8B"},
		{ID: "codellama:7b", Name: "Code Llama 7B"},
	},
}

func newModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model [model-id]",
		Short: "Show or set this project's model",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			if len(args) == 0 {
				s, ok := resolveEngineSettingsForDir(cfg, cwd)
				if !ok {
					return fmt.Errorf("no provider configured - run `lore config set` first")
				}
				projectModel := strings.TrimSpace(readLoreProjectConfig(cwd).Model)
				fmt.Println("effective model: " + emptyDash(s.model))
				fmt.Println("project model:   " + emptyDash(projectModel))
				fmt.Println("global model:    " + emptyDash(cfg.Engine.Model))
				fmt.Println(display.DimStyle.Render("Use `lore models` to list candidates, then `lore model <id>` to switch this project."))
				return nil
			}
			newModel := strings.TrimSpace(args[0])
			if newModel == "" {
				return fmt.Errorf("model id cannot be empty")
			}
			if _, err := ensureLoreWiki(cwd); err != nil {
				return fmt.Errorf("initializing .lore wiki: %w", err)
			}
			if err := setProjectModel(cwd, cfg, newModel); err != nil {
				return err
			}
			fmt.Println(display.DimStyle.Render("Project model set to " + newModel + "."))
			return nil
		},
	}
	return cmd
}

func newModelsCmd() *cobra.Command {
	var all bool
	var offline bool
	var limit int

	cmd := &cobra.Command{
		Use:   "models [query]",
		Short: "List models available for the active provider",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			cwd, _ := os.Getwd()
			s, ok := resolveEngineSettingsForDir(cfg, cwd)
			if !ok {
				return fmt.Errorf("no provider configured - run `lore config set` first")
			}
			models, fetchErr := availableModels(context.Background(), s, !offline)
			models = filterModels(models, query)
			if !all && limit > 0 && len(models) > limit {
				models = models[:limit]
			}
			fmt.Print(formatModelList(s, models, query, fetchErr))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "show the full live provider catalog")
	cmd.Flags().BoolVar(&offline, "offline", false, "use Lore's curated model list without contacting the provider")
	cmd.Flags().IntVar(&limit, "limit", 40, "maximum models to show unless --all is set")
	return cmd
}

func setProjectModel(cwd string, cfg *config.Config, model string) error {
	s, ok := resolveEngineSettingsForDir(cfg, cwd)
	if !ok {
		return fmt.Errorf("no provider configured - run `lore config set` first")
	}
	s.model = model
	if _, err := buildProvider(s); err != nil {
		return fmt.Errorf("invalid model: %w", err)
	}
	projectCfg := readLoreProjectConfig(cwd)
	projectCfg.Model = model
	writeLoreProjectConfig(cwd, projectCfg)
	return nil
}

func availableModels(ctx context.Context, s engineSettings, live bool) ([]modelInfo, error) {
	models := curatedProviderModels(s.provider.name)
	if !live || s.provider.anthropic {
		return models, nil
	}
	liveModels, err := fetchProviderModels(ctx, s)
	if err != nil {
		return models, err
	}
	return mergeModels(models, liveModels), nil
}

func curatedProviderModels(provider string) []modelInfo {
	src := curatedModels[provider]
	if len(src) == 0 {
		src = []modelInfo{{ID: "model-id", Name: "Custom model"}}
	}
	out := make([]modelInfo, 0, len(src))
	for _, m := range src {
		m.Provider = provider
		m.Source = "curated"
		out = append(out, m)
	}
	return out
}

func fetchProviderModels(ctx context.Context, s engineSettings) ([]modelInfo, error) {
	if strings.TrimSpace(s.baseURL) == "" {
		return nil, fmt.Errorf("provider has no base URL")
	}
	endpoint, err := modelsEndpoint(s.baseURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("live model fetch returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]modelInfo, 0, len(body.Data))
	for _, item := range body.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		out = append(out, modelInfo{
			ID:       id,
			Name:     strings.TrimSpace(item.Name),
			Provider: s.provider.name,
			Source:   "live",
		})
	}
	sortModels(out)
	return out, nil
}

func modelsEndpoint(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	return u.String(), nil
}

func mergeModels(curated, live []modelInfo) []modelInfo {
	seen := map[string]int{}
	out := make([]modelInfo, 0, len(curated)+len(live))
	for _, m := range curated {
		seen[strings.ToLower(m.ID)] = len(out)
		out = append(out, m)
	}
	for _, m := range live {
		key := strings.ToLower(m.ID)
		if idx, ok := seen[key]; ok {
			out[idx].Source = "live"
			if out[idx].Name == "" {
				out[idx].Name = m.Name
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, m)
	}
	sortModels(out)
	return out
}

func sortModels(models []modelInfo) {
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Recommended != models[j].Recommended {
			return models[i].Recommended
		}
		return strings.ToLower(models[i].ID) < strings.ToLower(models[j].ID)
	})
}

func filterModels(models []modelInfo, query string) []modelInfo {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return models
	}
	var out []modelInfo
	for _, m := range models {
		hay := strings.ToLower(m.ID + " " + m.Name)
		if strings.Contains(hay, query) {
			out = append(out, m)
		}
	}
	return out
}

func formatModelList(s engineSettings, models []modelInfo, query string, fetchErr error) string {
	var sb strings.Builder
	label := s.provider.label
	if label == "" {
		label = s.provider.name
	}
	fmt.Fprintf(&sb, "models for %s (%s)\n", label, s.provider.name)
	if query != "" {
		fmt.Fprintf(&sb, "filter: %s\n", query)
	}
	if fetchErr != nil {
		fmt.Fprintf(&sb, "%s\n", display.DimStyle.Render("live model fetch failed; showing curated list: "+fetchErr.Error()))
	}
	if len(models) == 0 {
		sb.WriteString("no models matched\n")
		return sb.String()
	}
	for _, m := range models {
		mark := " "
		if m.ID == s.model {
			mark = "*"
		} else if m.Recommended {
			mark = "+"
		}
		name := m.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(&sb, "%s %-38s  %-24s  %s\n", mark, m.ID, name, m.Source)
	}
	sb.WriteString(display.DimStyle.Render("Use `lore model <id>` to set this project, or `lore config model <id>` for the global default.") + "\n")
	return sb.String()
}
