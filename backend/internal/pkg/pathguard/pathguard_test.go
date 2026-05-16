package pathguard

import (
	"os"
	"path/filepath"
	"testing"
)

func homeDir(t *testing.T) string {
	t.Helper()
	h, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir unavailable: %v", err)
	}
	return h
}

func TestDefault_DeniesUserCredentials(t *testing.T) {
	g := NewDefault()
	home := homeDir(t)
	cases := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".gnupg", "private-keys-v1.d"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".config", "git-credentials"),
	}
	for _, p := range cases {
		ok, reason := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied, but was allowed", p)
		}
		if reason == "" {
			t.Errorf("expected non-empty reason for denied path %q", p)
		}
	}
}

func TestDefault_DeniesSystemPaths(t *testing.T) {
	g := NewDefault()
	cases := []string{
		"/etc/hosts",
		"/etc/passwd",
		"/usr/bin/ls",
		"/bin/sh",
		"/private/etc/hosts",
		"/System/Library/Frameworks",
	}
	for _, p := range cases {
		ok, _ := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied, but was allowed", p)
		}
	}
}

func TestDefault_DeniesForgifyState(t *testing.T) {
	g := NewDefault()
	home := homeDir(t)
	cases := []string{
		filepath.Join(home, ".forgify"),
		filepath.Join(home, ".forgify", "db.sqlite"),
		filepath.Join(home, ".forgify", "encryption-key"),
		filepath.Join(home, ".forgify", "forges", "f_abc"),
	}
	for _, p := range cases {
		ok, _ := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied, but was allowed", p)
		}
	}
}

func TestDefault_DeniesLinuxRuntimePaths(t *testing.T) {
	g := NewDefault()
	cases := []string{
		"/proc/1/environ",
		"/proc/self/maps",
		"/run/secrets/kubernetes.io/serviceaccount/token",
		"/var/run/secrets/kubernetes.io/serviceaccount/token",
		"/sys/class/net/eth0/address",
	}
	for _, p := range cases {
		ok, _ := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied (Linux runtime), but was allowed", p)
		}
	}
}

func TestDefault_DeniesWindowsCredentialPaths(t *testing.T) {
	g := NewDefault()
	home := homeDir(t)
	cases := []string{
		filepath.Join(home, "AppData", "Roaming", "Microsoft", "Credentials", "abc"),
		filepath.Join(home, "AppData", "Local", "Microsoft", "Credentials", "xyz"),
		filepath.Join(home, "AppData", "Roaming", "Microsoft", "Crypto", "RSA", "key"),
		filepath.Join(home, "AppData", "Roaming", "Microsoft", "Protect", "S-1-5-21-x"),
		filepath.Join(home, "AppData", "Local", "Microsoft", "Vault", "x.vsch"),
	}
	for _, p := range cases {
		ok, _ := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied (Windows credential store), but was allowed", p)
		}
	}
}

func TestDefault_DeniesBrowserLoginPaths(t *testing.T) {
	g := NewDefault()
	home := homeDir(t)
	cases := []string{
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Login Data"),
		filepath.Join(home, ".config", "google-chrome", "Default", "Login Data"),
		filepath.Join(home, "AppData", "Local", "Google", "Chrome", "User Data", "Default", "Login Data"),
		filepath.Join(home, "AppData", "Local", "Microsoft", "Edge", "User Data", "Default", "Login Data"),
	}
	for _, p := range cases {
		ok, _ := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied (browser logins), but was allowed", p)
		}
	}
}

func TestDefault_DeniesKubeAndDockerConfig(t *testing.T) {
	g := NewDefault()
	home := homeDir(t)
	cases := []string{
		filepath.Join(home, ".docker", "config.json"),
		filepath.Join(home, ".kube", "config"),
	}
	for _, p := range cases {
		ok, _ := g.Allow(p)
		if ok {
			t.Errorf("expected %q to be denied, but was allowed", p)
		}
	}
}

func TestDefault_AllowsNormalPaths(t *testing.T) {
	g := NewDefault()
	home := homeDir(t)
	cases := []string{
		filepath.Join(home, "Documents", "report.md"),
		filepath.Join(home, "Downloads", "data.csv"),
		filepath.Join(home, "Desktop", "notes.txt"),
		filepath.Join(home, "Projects", "myrepo", "main.go"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}
	for _, p := range cases {
		ok, reason := g.Allow(p)
		if !ok {
			t.Errorf("expected %q to be allowed, was denied: %s", p, reason)
		}
	}
}

func TestAllow_RejectsRelativePath(t *testing.T) {
	g := NewDefault()
	cases := []string{
		"foo.txt",
		"./bar.txt",
		"some/relative/path",
		"",
	}
	for _, p := range cases {
		ok, reason := g.Allow(p)
		if ok {
			t.Errorf("expected relative path %q to be rejected", p)
		}
		if reason == "" {
			t.Errorf("expected non-empty reason for relative path %q", p)
		}
	}
}

func TestNew_DirectoryRuleMatchesPathItselfAndDescendants(t *testing.T) {
	g := New([]string{"/secret/"})

	if ok, _ := g.Allow("/secret"); ok {
		t.Errorf("expected /secret (the dir itself) to be denied")
	}
	if ok, _ := g.Allow("/secret/key.pem"); ok {
		t.Errorf("expected /secret/key.pem to be denied")
	}
	if ok, _ := g.Allow("/secret/sub/deep/file"); ok {
		t.Errorf("expected /secret/sub/deep/file to be denied")
	}
	if ok, _ := g.Allow("/secretly/safe.txt"); !ok {
		t.Errorf("expected /secretly/safe.txt to be allowed (not under /secret/)")
	}
}

func TestNew_FileRuleExactMatchOnly(t *testing.T) {
	g := New([]string{"/etc/important.conf"})

	if ok, _ := g.Allow("/etc/important.conf"); ok {
		t.Errorf("expected exact /etc/important.conf to be denied")
	}
	if ok, _ := g.Allow("/etc/important.conf.bak"); !ok {
		t.Errorf("expected /etc/important.conf.bak to be allowed (not exact match)")
	}
	if ok, _ := g.Allow("/etc/other.conf"); !ok {
		t.Errorf("expected /etc/other.conf to be allowed")
	}
}

func TestNew_TildeExpansion(t *testing.T) {
	g := New([]string{"~/.secrets/"})
	home := homeDir(t)

	denied := filepath.Join(home, ".secrets", "key")
	if ok, _ := g.Allow(denied); ok {
		t.Errorf("expected %q to be denied via ~ expansion", denied)
	}

	allowed := filepath.Join(home, ".other", "file")
	if ok, _ := g.Allow(allowed); !ok {
		t.Errorf("expected %q to be allowed", allowed)
	}
}

func TestNew_PathCleanResolvesTraversal(t *testing.T) {
	g := New([]string{"/forbidden/"})

	if ok, _ := g.Allow("/forbidden/../forbidden/x"); ok {
		t.Errorf("expected /forbidden/../forbidden/x to be denied after Clean")
	}
	if ok, _ := g.Allow("/safe/../forbidden/x"); ok {
		t.Errorf("expected /safe/../forbidden/x to be denied after Clean")
	}
	if ok, _ := g.Allow("/forbidden/../safe/x"); !ok {
		t.Errorf("expected /forbidden/../safe/x to be allowed after Clean (resolves to /safe/x)")
	}
}

func TestNew_EmptyDenyListAllowsEverything(t *testing.T) {
	g := New([]string{})

	cases := []string{
		"/etc/hosts",
		"/usr/bin/anything",
		filepath.Join(homeDir(t), ".ssh", "id_rsa"),
	}
	for _, p := range cases {
		if ok, _ := g.Allow(p); !ok {
			t.Errorf("expected %q to be allowed by empty deny list", p)
		}
	}
}

func TestNew_NonAbsoluteRuleSilentlyDropped(t *testing.T) {
	g := New([]string{"relative/path/"})

	if ok, _ := g.Allow("/etc/hosts"); !ok {
		t.Errorf("relative rule should be dropped; /etc/hosts should still be allowed")
	}
}
