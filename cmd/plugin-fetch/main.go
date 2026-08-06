package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/fetcher"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

// Environment variables
const (
	InputFileEnvVar       = "INPUT_FILE"       // Package list file (default: /input/packages.txt)
	OutputDirEnvVar       = "OUTPUT_DIR"       // Output directory (default: /dynamic-plugins-root)
	DockerConfigEnvVar    = "DOCKER_CONFIG"    // Path to dockerconfigjson file
	CAFileEnvVar          = "CA_FILE"          // Path to CA certificate file
	InsecureEnvVar        = "INSECURE"         // Skip TLS verification (true/false)
	ValidatePluginsEnvVar = "VALIDATE_PLUGINS" // Validate plugin artifacts (default: true)
	ParallelEnvVar        = "PARALLEL"         // Number of parallel downloads (default: 4)
	LockFileEnvVar        = "LOCK_FILE"        // Lock file path (default: OUTPUT_DIR/install-dynamic-plugins.lock)
	TerminationLogEnvVar  = "TERMINATION_LOG"  // Kubernetes termination log

	// Catalog index for Extensions UI
	CatalogIndexImageEnvVar         = "CATALOG_INDEX_IMAGE"          // OCI image containing catalog entities
	CatalogEntitiesExtractDirEnvVar = "CATALOG_ENTITIES_EXTRACT_DIR" // Where to extract catalog entities (default: /tmp/extensions)

	// NPM configuration
	SkipIntegrityCheckEnvVar = "SKIP_INTEGRITY_CHECK"  // Skip integrity verification (default: false)
	NPMRegistryEnvVar        = "NPM_REGISTRY"          // NPM registry URL (default: https://registry.npmjs.org)
	NPMAuthTokenEnvVar       = "NPM_AUTH_TOKEN"        // NPM auth token
	NPMConfigEnvVar          = "NPM_CONFIG_USERCONFIG" // Path to .npmrc file
)

// Package represents a plugin package with optional integrity hash
type Package struct {
	URL       string
	Integrity string
}

func main() {
	// Read configuration from environment
	inputFile := utils.StringEnvVar(InputFileEnvVar, "/input/packages.txt")
	outputDir := utils.StringEnvVar(OutputDirEnvVar, "/dynamic-plugins-root")
	dockerConfig := utils.StringEnvVar(DockerConfigEnvVar, "")
	caFile := utils.StringEnvVar(CAFileEnvVar, "")
	insecure := utils.BoolEnvVar(InsecureEnvVar, false)
	validate := utils.BoolEnvVar(ValidatePluginsEnvVar, true)
	parallel := utils.IntEnvVar(ParallelEnvVar, 4)
	lockFile := utils.StringEnvVar(LockFileEnvVar, filepath.Join(outputDir, "install-dynamic-plugins.lock"))
	termLog := utils.StringEnvVar(TerminationLogEnvVar, "/dev/termination-log")

	// Catalog index settings
	catalogIndexImage := utils.StringEnvVar(CatalogIndexImageEnvVar, "")
	catalogEntitiesDir := utils.StringEnvVar(CatalogEntitiesExtractDirEnvVar, "/tmp/extensions")

	// NPM settings
	skipIntegrityCheck := utils.BoolEnvVar(SkipIntegrityCheckEnvVar, false)
	npmRegistry := utils.StringEnvVar(NPMRegistryEnvVar, "")
	npmAuthToken := utils.StringEnvVar(NPMAuthTokenEnvVar, "")
	npmConfigFile := utils.StringEnvVar(NPMConfigEnvVar, "")

	// Handle signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	// Ensure output directory exists (needed for lock file)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fatal(termLog, "Failed to create output directory: %v", err)
	}

	// Acquire lock
	lock := fetcher.NewFileLock(lockFile)
	if err := lock.Acquire(5 * time.Minute); err != nil {
		fatal(termLog, "Failed to acquire lock: %v", err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to release lock: %v\n", err)
		}
	}()

	// Build OCI options
	var ociOpts []fetcher.OCIOption
	if insecure {
		ociOpts = append(ociOpts, fetcher.WithInsecure())
	}
	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			fatal(termLog, "Error reading CA file: %v", err)
		}
		ociOpts = append(ociOpts, fetcher.WithCACert(caCert))
	}
	if dockerConfig != "" {
		secret, err := os.ReadFile(dockerConfig)
		if err != nil {
			fatal(termLog, "Error reading docker config: %v", err)
		}
		ociOpts = append(ociOpts, fetcher.WithDockerConfig(secret))
	}
	if validate {
		ociOpts = append(ociOpts, fetcher.WithPluginValidation())
	}

	// Build NPM options
	var npmOpts []fetcher.NPMOption
	if npmRegistry != "" {
		npmOpts = append(npmOpts, fetcher.WithNPMRegistry(npmRegistry))
	}
	if npmAuthToken != "" {
		npmOpts = append(npmOpts, fetcher.WithNPMAuthToken(npmAuthToken))
	}
	if npmConfigFile != "" {
		npmOpts = append(npmOpts, fetcher.WithNPMConfigFile(npmConfigFile))
	}
	if skipIntegrityCheck {
		npmOpts = append(npmOpts, fetcher.WithSkipIntegrityCheck())
	}

	// Create fetcher
	f := fetcher.New(
		fetcher.WithOCIOptions(ociOpts...),
		fetcher.WithNPMOptions(npmOpts...),
	)

	// Extract catalog entities from catalog index image (for Extensions UI)
	if catalogIndexImage != "" {
		fmt.Printf("=== Extracting catalog entities from %s ===\n", catalogIndexImage)
		if err := extractCatalogEntities(ctx, f, catalogIndexImage, catalogEntitiesDir, ociOpts); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Failed to extract catalog index: %v\n", err)
		}
	} else {
		fmt.Println("=== CATALOG_INDEX_IMAGE not set, skipping catalog entities extraction")
	}

	// Read packages from input file
	packages, err := readPackages(inputFile)
	if err != nil {
		fatal(termLog, "Error reading input file: %v", err)
	}

	if len(packages) == 0 {
		fmt.Println("No packages to install")
		return
	}

	fmt.Printf("=== Downloading %d plugins to %s (%d parallel) ===\n\n", len(packages), outputDir, parallel)

	// Process packages in parallel
	if err := processParallel(ctx, f, packages, outputDir, parallel); err != nil {
		fatal(termLog, "Error installing plugins: %v", err)
	}

	fmt.Printf("\n=== Complete ===\n")
	fmt.Printf("Successfully installed %d plugins\n", len(packages))
}

// extractCatalogEntities extracts catalog-entities from an OCI image
func extractCatalogEntities(ctx context.Context, f *fetcher.Fetcher, image, destDir string, ociOpts []fetcher.OCIOption) error {
	// Create OCI fetcher without plugin validation (catalog index is not a plugin)
	ociFetcher := fetcher.NewOCIFetcher(ociOpts...)

	tmpDir, err := os.MkdirTemp("", "catalog-index-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Fetch and extract OCI image
	if err := ociFetcher.Fetch(ctx, strings.TrimPrefix(image, "oci://"), tmpDir); err != nil {
		return fmt.Errorf("failed to fetch catalog index: %w", err)
	}

	// Look for catalog-entities/extensions or catalog-entities
	var entitiesSrc string
	if info, err := os.Stat(filepath.Join(tmpDir, "catalog-entities", "extensions")); err == nil && info.IsDir() {
		entitiesSrc = filepath.Join(tmpDir, "catalog-entities", "extensions")
	} else if info, err := os.Stat(filepath.Join(tmpDir, "catalog-entities")); err == nil && info.IsDir() {
		entitiesSrc = filepath.Join(tmpDir, "catalog-entities")
	}

	if entitiesSrc == "" {
		return fmt.Errorf("no catalog-entities found in %s", image)
	}

	// Copy to destination
	entitiesDest := filepath.Join(destDir, "catalog-entities")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	os.RemoveAll(entitiesDest) // Remove old if exists

	if err := copyDir(entitiesSrc, entitiesDest); err != nil {
		return fmt.Errorf("failed to copy catalog entities: %w", err)
	}

	// Count YAML files
	count := 0
	filepath.Walk(entitiesDest, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			count++
		}
		return nil
	})

	fmt.Printf("Catalog entities extracted to %s (%d files)\n", entitiesDest, count)
	return nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func readPackages(path string) ([]Package, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var packages []Package
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Handle "url [integrity]" format
		parts := strings.Fields(line)
		pkg := Package{URL: parts[0]}
		if len(parts) > 1 {
			pkg.Integrity = parts[1]
		}
		packages = append(packages, pkg)
	}
	return packages, scanner.Err()
}

func processParallel(ctx context.Context, f *fetcher.Fetcher, packages []Package, outputDir string, parallel int) error {
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for _, pkg := range packages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(pkg Package) {
			defer wg.Done()
			defer func() { <-sem }()

			name := pluginName(pkg.URL)
			destDir := filepath.Join(outputDir, name)

			// Skip if already exists
			if info, err := os.Stat(destDir); err == nil && info.IsDir() {
				entries, _ := os.ReadDir(destDir)
				if len(entries) > 0 {
					fmt.Printf("[SKIP] %s (exists)\n", name)
					return
				}
			}

			fmt.Printf("[DOWN] %s\n", pkg.URL)

			if err := f.FetchWithIntegrity(ctx, pkg.URL, destDir, pkg.Integrity); err != nil {
				fmt.Fprintf(os.Stderr, "[FAIL] %s: %v\n", name, err)
				errOnce.Do(func() {
					firstErr = fmt.Errorf("%s: %w", name, err)
				})
				return
			}
			fmt.Printf("[DONE] %s\n", name)
		}(pkg)
	}

	wg.Wait()
	return firstErr
}

// pluginName extracts a directory name from various URL formats
func pluginName(url string) string {
	// Remove scheme prefixes
	url = strings.TrimPrefix(url, "oci://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "file://")
	url = strings.TrimPrefix(url, "file:")
	url = strings.TrimPrefix(url, "@npm:")

	// Remove digest/tag suffix
	if idx := strings.Index(url, "@sha256:"); idx > 0 {
		url = url[:idx]
	}
	if idx := strings.LastIndex(url, "@"); idx > 0 {
		// For npm packages like @backstage/plugin-foo@1.0.0
		// Keep @backstage/plugin-foo, remove @1.0.0
		beforeAt := url[:idx]
		if !strings.HasPrefix(beforeAt, "@") || strings.Contains(beforeAt[1:], "/") {
			url = beforeAt
		}
	}
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		// Remove tag like :latest or :v1.0.0
		url = url[:idx]
	}

	// Get the last path component
	name := filepath.Base(url)

	// Clean up common suffixes
	name = strings.TrimSuffix(name, ".tgz")
	name = strings.TrimSuffix(name, ".tar.gz")
	name = strings.TrimSuffix(name, ".tar")

	return name
}

func writeTerminationLog(path, msg string) {
	if len(msg) > 4096 {
		msg = msg[:4096]
	}
	_ = os.WriteFile(path, []byte(msg), 0644)
}

func fatal(termLog, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	writeTerminationLog(termLog, msg)
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
