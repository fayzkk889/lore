package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateContractWritesProjectArtifact(t *testing.T) {
	dir := t.TempDir()
	if _, err := ensureLoreWiki(dir); err != nil {
		t.Fatal(err)
	}

	contract, err := createContract(dir, "Fix login redirect", []string{"redirects after login"}, []string{"echo contract-ok"}, []string{"cmd/"}, []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if contract.ID == "" {
		t.Fatal("contract ID is empty")
	}
	if contract.Status != "draft" {
		t.Fatalf("status = %q, want draft", contract.Status)
	}
	if _, err := os.Stat(contractPath(dir, contract.ID)); err != nil {
		t.Fatalf("contract file missing: %v", err)
	}

	loaded, err := loadContract(dir, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != contract.ID {
		t.Fatalf("latest ID = %q, want %q", loaded.ID, contract.ID)
	}
	if !strings.Contains(formatContract(loaded), "redirects after login") {
		t.Fatal("formatted contract does not include success criteria")
	}
}

func TestContractReplayAppendsProof(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := ensureLoreWiki(dir); err != nil {
		t.Fatal(err)
	}

	contract, err := createContract(dir, "Prove shell check", nil, []string{"echo contract-ok"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newContractCmd()
	cmd.SetArgs([]string{"replay", contract.ID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	loaded, err := loadContract(dir, contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "passed" {
		t.Fatalf("status = %q, want passed", loaded.Status)
	}
	if len(loaded.Proofs) != 1 {
		t.Fatalf("proof count = %d, want 1", len(loaded.Proofs))
	}
	if !strings.Contains(loaded.Proofs[0].Detail, "contract-ok") {
		t.Fatalf("proof detail = %q, want contract output", loaded.Proofs[0].Detail)
	}
}

func TestEnsureLoreWikiCreatesContractsDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := ensureLoreWiki(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, ".lore", "contracts"))
	if err != nil {
		t.Fatalf("contracts dir missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("contracts path is not a directory")
	}
}
