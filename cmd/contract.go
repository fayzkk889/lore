package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/display"
	"github.com/fayzkk889/lore/internal/lorefs"
	"github.com/fayzkk889/lore/internal/verify"
)

type loreContract struct {
	ID              string          `json:"id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Status          string          `json:"status"`
	Intent          string          `json:"intent"`
	SuccessCriteria []string        `json:"success_criteria,omitempty"`
	AllowedPaths    []string        `json:"allowed_paths,omitempty"`
	ForbiddenPaths  []string        `json:"forbidden_paths,omitempty"`
	Checks          []verify.Check  `json:"checks,omitempty"`
	Proofs          []contractProof `json:"proofs,omitempty"`
}

type contractProof struct {
	Time         time.Time `json:"time"`
	Kind         string    `json:"kind"`
	Passed       bool      `json:"passed"`
	Detail       string    `json:"detail,omitempty"`
	Engine       string    `json:"engine,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
}

func newContractCmd() *cobra.Command {
	var success []string
	var checks []string
	var allow []string
	var deny []string

	cmd := &cobra.Command{
		Use:   "contract \"<intent>\"",
		Short: "Create, inspect, and replay executable work contracts",
		Long:  "Contracts capture intent, success criteria, scope, verification checks, and proof for a piece of agent work.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return listContractsCmd(10)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			if _, err := ensureLoreWiki(cwd); err != nil {
				return fmt.Errorf("initializing .lore wiki: %w", err)
			}
			contract, err := createContract(cwd, args[0], success, checks, allow, deny)
			if err != nil {
				return err
			}
			fmt.Println(display.SuccessStyle.Render("Contract created: ") + contract.ID)
			fmt.Println(contractPath(cwd, contract.ID))
			fmt.Println(display.DimStyle.Render("Run `lore contract show " + contract.ID + "` to inspect it, or `lore do --contract " + contract.ID + " \"...\"` to attach agent proof."))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&success, "success", nil, "success criterion; repeat for multiple criteria")
	cmd.Flags().StringArrayVar(&checks, "check", nil, "CLI verification command; repeat for multiple checks")
	cmd.Flags().StringArrayVar(&allow, "allow", nil, "path or glob that is in scope; repeat for multiple paths")
	cmd.Flags().StringArrayVar(&deny, "deny", nil, "path or glob that must not change; repeat for multiple paths")

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List recent contracts",
		RunE: func(_ *cobra.Command, _ []string) error {
			return listContractsCmd(20)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show [id]",
		Short: "Show a contract, defaulting to the latest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			id := "latest"
			if len(args) > 0 {
				id = args[0]
			}
			contract, err := loadContract(cwd, id)
			if err != nil {
				return err
			}
			fmt.Println(formatContract(contract))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "replay [id]",
		Short: "Rerun a contract's verification checks and append proof",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			id := "latest"
			if len(args) > 0 {
				id = args[0]
			}
			contract, err := loadContract(cwd, id)
			if err != nil {
				return err
			}
			result := verify.Run(verify.Options{Dir: cwd, Checks: contract.Checks})
			detail := result.Summary()
			contract.addProof(contractProof{
				Time:   time.Now(),
				Kind:   "replay",
				Passed: result.Passed,
				Detail: detail,
			})
			if err := writeContract(cwd, contract); err != nil {
				return err
			}
			fmt.Print(detail)
			if !result.Passed {
				return fmt.Errorf("contract replay failed: %s", contract.ID)
			}
			return nil
		},
	})
	return cmd
}

func createContract(cwd, intent string, success, checkCommands, allow, deny []string) (loreContract, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return loreContract{}, fmt.Errorf("contract intent cannot be empty")
	}
	now := time.Now()
	c := loreContract{
		ID:              contractID(now, intent),
		CreatedAt:       now,
		UpdatedAt:       now,
		Status:          "draft",
		Intent:          intent,
		SuccessCriteria: cleanStrings(success),
		AllowedPaths:    cleanStrings(allow),
		ForbiddenPaths:  cleanStrings(deny),
	}
	for _, command := range cleanStrings(checkCommands) {
		c.Checks = append(c.Checks, verify.Check{Type: "cli", Command: command})
	}
	if len(c.SuccessCriteria) == 0 {
		c.SuccessCriteria = []string{"Implementation satisfies the stated intent.", "Verification passes without regressions."}
	}
	if err := writeContract(cwd, c); err != nil {
		return loreContract{}, err
	}
	return c, nil
}

func (c *loreContract) addProof(proof contractProof) {
	c.Proofs = append(c.Proofs, proof)
	c.UpdatedAt = proof.Time
	if proof.Passed {
		c.Status = "passed"
	} else {
		c.Status = "failed"
	}
}

func contractID(t time.Time, intent string) string {
	slug := strings.ToLower(intent)
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "work"
	}
	if len(slug) > 42 {
		slug = strings.Trim(slug[:42], "-")
	}
	return t.Format("20060102-150405") + "-" + slug
}

func contractDir(cwd string) string {
	return filepath.Join(cwd, ".lore", "contracts")
}

func contractPath(cwd, id string) string {
	return filepath.Join(contractDir(cwd), id+".json")
}

func writeContract(cwd string, c loreContract) error {
	if err := lorefs.MkdirPrivate(contractDir(cwd)); err != nil {
		return fmt.Errorf("creating contracts directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return lorefs.WritePrivate(contractPath(cwd, c.ID), data)
}

func loadContract(cwd, id string) (loreContract, error) {
	contracts, err := listContracts(cwd)
	if err != nil {
		return loreContract{}, err
	}
	if len(contracts) == 0 {
		return loreContract{}, fmt.Errorf("no contracts found")
	}
	id = strings.TrimSpace(id)
	if id == "" || id == "latest" {
		return contracts[0], nil
	}
	for _, c := range contracts {
		if c.ID == id || strings.HasPrefix(c.ID, id) {
			return c, nil
		}
	}
	return loreContract{}, fmt.Errorf("contract %q not found", id)
}

func listContracts(cwd string) ([]loreContract, error) {
	entries, err := os.ReadDir(contractDir(cwd))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var contracts []loreContract
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(contractDir(cwd), entry.Name()))
		if err != nil {
			continue
		}
		var c loreContract
		if json.Unmarshal(data, &c) == nil && c.ID != "" {
			contracts = append(contracts, c)
		}
	}
	sort.Slice(contracts, func(i, j int) bool {
		return contracts[i].UpdatedAt.After(contracts[j].UpdatedAt)
	})
	return contracts, nil
}

func listContractsCmd(limit int) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	contracts, err := listContracts(cwd)
	if err != nil {
		return err
	}
	if len(contracts) == 0 {
		fmt.Println("No contracts yet. Create one with `lore contract \"<intent>\"`.")
		return nil
	}
	if len(contracts) > limit {
		contracts = contracts[:limit]
	}
	for _, c := range contracts {
		fmt.Printf("%s  %-7s  %s\n", c.ID, c.Status, trimTo(c.Intent, 80))
	}
	return nil
}

func formatContract(c loreContract) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "contract: %s\n", c.ID)
	fmt.Fprintf(&sb, "status:   %s\n", c.Status)
	fmt.Fprintf(&sb, "intent:   %s\n", c.Intent)
	if len(c.SuccessCriteria) > 0 {
		sb.WriteString("\nsuccess criteria:\n")
		for _, item := range c.SuccessCriteria {
			fmt.Fprintf(&sb, "  - %s\n", item)
		}
	}
	if len(c.AllowedPaths) > 0 {
		sb.WriteString("\nallowed paths:\n")
		for _, item := range c.AllowedPaths {
			fmt.Fprintf(&sb, "  - %s\n", item)
		}
	}
	if len(c.ForbiddenPaths) > 0 {
		sb.WriteString("\nforbidden paths:\n")
		for _, item := range c.ForbiddenPaths {
			fmt.Fprintf(&sb, "  - %s\n", item)
		}
	}
	if len(c.Checks) > 0 {
		sb.WriteString("\nchecks:\n")
		for _, check := range c.Checks {
			fmt.Fprintf(&sb, "  - %s\n", check.Command)
		}
	}
	if len(c.Proofs) > 0 {
		sb.WriteString("\nproofs:\n")
		for _, proof := range c.Proofs {
			status := "FAIL"
			if proof.Passed {
				status = "PASS"
			}
			fmt.Fprintf(&sb, "  - %s %s %s\n", proof.Time.Format("2006-01-02 15:04:05"), status, proof.Kind)
			if proof.Engine != "" {
				fmt.Fprintf(&sb, "    engine: %s\n", proof.Engine)
			}
			if proof.Detail != "" {
				fmt.Fprintf(&sb, "    %s\n", trimTo(strings.ReplaceAll(proof.Detail, "\n", " | "), 160))
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func contractContext(c loreContract) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### Active Lore contract: %s\n", c.ID)
	fmt.Fprintf(&sb, "Intent: %s\n", c.Intent)
	if len(c.SuccessCriteria) > 0 {
		sb.WriteString("Success criteria:\n")
		for _, item := range c.SuccessCriteria {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}
	if len(c.AllowedPaths) > 0 {
		sb.WriteString("Allowed paths:\n")
		for _, item := range c.AllowedPaths {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}
	if len(c.ForbiddenPaths) > 0 {
		sb.WriteString("Forbidden paths:\n")
		for _, item := range c.ForbiddenPaths {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}
	if len(c.Checks) > 0 {
		sb.WriteString("Required contract checks:\n")
		for _, check := range c.Checks {
			fmt.Fprintf(&sb, "- %s\n", check.Command)
		}
	}
	sb.WriteString("Before claiming completion, satisfy this contract and preserve proof in the final response.\n")
	return sb.String()
}

func cleanStrings(items []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
