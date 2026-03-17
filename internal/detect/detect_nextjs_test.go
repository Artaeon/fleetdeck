package detect

import (
	"testing"
)

func TestNextJS_StandaloneConfigNoWarning(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "package.json", `{
		"name": "test-app",
		"dependencies": {
			"next": "14.0.0",
			"react": "18.0.0"
		}
	}`)
	writeFile(t, dir, "next.config.js", `
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
}
module.exports = nextConfig
`)

	r, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}

	if r.AppType != AppTypeNextJS {
		t.Fatalf("expected AppType nextjs, got %s", r.AppType)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("expected no warnings with standalone config, got: %v", r.Warnings)
	}

	found := false
	for _, ind := range r.Indicators {
		if ind == "standalone output configured" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'standalone output configured' indicator")
	}
}

func TestNextJS_NoStandaloneConfigWarning(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "package.json", `{
		"name": "test-app",
		"dependencies": {
			"next": "14.0.0",
			"react": "18.0.0"
		}
	}`)
	writeFile(t, dir, "next.config.js", `
/** @type {import('next').NextConfig} */
const nextConfig = {}
module.exports = nextConfig
`)

	r, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}

	if r.AppType != AppTypeNextJS {
		t.Fatalf("expected AppType nextjs, got %s", r.AppType)
	}
	if len(r.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(r.Warnings), r.Warnings)
	}

	expected := `Next.js standalone output not detected; add output: "standalone" to next.config for optimized Docker builds`
	if r.Warnings[0] != expected {
		t.Fatalf("unexpected warning:\n  got:  %s\n  want: %s", r.Warnings[0], expected)
	}
}

func TestNextJS_PrismaSetsHasDB(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "package.json", `{
		"name": "test-app",
		"dependencies": {
			"next": "14.0.0",
			"react": "18.0.0",
			"@prisma/client": "5.0.0"
		}
	}`)
	writeFile(t, dir, "next.config.mjs", `
export default {
  output: 'standalone',
}
`)

	r, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}

	if r.AppType != AppTypeNextJS {
		t.Fatalf("expected AppType nextjs, got %s", r.AppType)
	}
	if !r.HasDB {
		t.Fatal("expected HasDB to be true with @prisma/client dependency")
	}

	found := false
	for _, ind := range r.Indicators {
		if ind == "uses Prisma (database)" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'uses Prisma (database)' indicator")
	}
}
