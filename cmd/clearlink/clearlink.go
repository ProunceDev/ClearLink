package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	GitHubOwner = "ProunceDev"
	GitHubRepo  = "ClearLink"
	BuildBranch = "builds"

	DefaultUpdateInterval = 5 * time.Second
	ShutdownTimeout       = 10 * time.Second
)

type Config struct {
	LinkType string `json:"link_type"`

	ServerAddress string `json:"server_address,omitempty"`
	Frequency     int64  `json:"frequency,omitempty"`
	Device        int    `json:"device,omitempty"`

	AutoUpdate     bool          `json:"auto_update"`
	UpdateInterval time.Duration `json:"update_interval"`

	Version string `json:"version"`
}

type LatestBuild struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	UpdatedAt string `json:"updated_at"`

	Platforms map[string]string `json:"platforms"`
}

type Manager struct {
	config Config

	configPath string
	baseDir    string
	currentDir string
	versionDir string

	child *exec.Cmd
}

func main() {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("           ClearLink Manager")
	fmt.Println("========================================")
	fmt.Println()

	if runtime.GOOS != "linux" &&
		runtime.GOOS != "windows" {

		fatal(
			"unsupported operating system: " +
				runtime.GOOS,
		)
	}

	if runtime.GOOS == "linux" &&
		runtime.GOARCH != "amd64" &&
		runtime.GOARCH != "arm64" {

		fatal(
			"unsupported Linux architecture: " +
				runtime.GOARCH,
		)
	}

	if runtime.GOOS == "windows" &&
		runtime.GOARCH != "amd64" {

		fatal(
			"unsupported Windows architecture: " +
				runtime.GOARCH,
		)
	}

	manager := newManager()

	if err := manager.ensureDirectories(); err != nil {
		fatal(err.Error())
	}

	if !manager.configExists() {
		if err := manager.firstRunSetup(); err != nil {
			fatal(err.Error())
		}
	} else {
		if err := manager.loadConfig(); err != nil {
			fatal(err.Error())
		}

		fmt.Println("Configuration loaded.")
		fmt.Println(
			"Link type:",
			manager.config.LinkType,
		)
		fmt.Println(
			"Installed version:",
			displayVersion(
				manager.config.Version,
			),
		)
		fmt.Println()
	}

	fmt.Println("Checking ClearLink installation...")

	if err := manager.ensureInstalled(); err != nil {
		fatal(err.Error())
	}

	fmt.Println()

	if err := manager.run(); err != nil {
		fatal(err.Error())
	}
}

func newManager() *Manager {
	baseDir := defaultBaseDir()

	return &Manager{
		baseDir: baseDir,

		configPath: filepath.Join(
			baseDir,
			"config.json",
		),

		currentDir: filepath.Join(
			baseDir,
			"current",
		),

		versionDir: filepath.Join(
			baseDir,
			"versions",
		),
	}
}

func defaultBaseDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")

		if base == "" {
			base = `C:\ProgramData`
		}

		return filepath.Join(
			base,
			"ClearLink",
		)
	}

	return "/opt/clearlink"
}

func (m *Manager) ensureDirectories() error {
	for _, dir := range []string{
		m.baseDir,
		m.currentDir,
		m.versionDir,
	} {
		if err := os.MkdirAll(
			dir,
			0755,
		); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) configExists() bool {
	_, err := os.Stat(m.configPath)

	return err == nil
}

func (m *Manager) firstRunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(
		"This appears to be the first time ClearLink has been run.",
	)
	fmt.Println()
	fmt.Println(
		"Let's configure your ClearLink installation.",
	)
	fmt.Println()

	var config Config

	fmt.Println("What type of link are you setting up?")
	fmt.Println()
	fmt.Println("  1. Broadcast")
	fmt.Println("  2. Listen")
	fmt.Println("  3. Server")
	fmt.Println("  4. Broadcast + Listen")
	fmt.Println()

	choice, err := promptNumber(
		reader,
		"Select [1-4]: ",
		1,
		4,
	)

	if err != nil {
		return err
	}

	switch choice {
	case 1:
		config.LinkType = "broadcast"

	case 2:
		config.LinkType = "listen"

	case 3:
		config.LinkType = "server"

	case 4:
		config.LinkType = "broadcast-listen"
	}

	fmt.Println()

	if config.LinkType == "listen" ||
		config.LinkType == "broadcast-listen" {

		fmt.Println(
			"RTL-SDR device number.",
		)

		device, err := promptNumber(
			reader,
			"Device [0]: ",
			0,
			100,
		)

		if err != nil {
			return err
		}

		config.Device = device

		fmt.Println()
	}

	if config.LinkType == "listen" ||
		config.LinkType == "broadcast" ||
		config.LinkType == "broadcast-listen" {

		fmt.Println(
			"Frequency configuration.",
		)

		fmt.Print(
			"Frequency in Hz [433920000]: ",
		)

		input, err := reader.ReadString('\n')

		if err != nil {
			return err
		}

		input = strings.TrimSpace(input)

		if input == "" {
			config.Frequency = 433920000
		} else {
			value, err := strconv.ParseInt(
				input,
				10,
				64,
			)

			if err != nil {
				return err
			}

			config.Frequency = value
		}

		fmt.Println()
	}

	if config.LinkType == "server" ||
		config.LinkType == "listen" ||
		config.LinkType == "broadcast-listen" {

		fmt.Println(
			"Server configuration.",
		)

		fmt.Print(
			"Server address [127.0.0.1:4125]: ",
		)

		input, err := reader.ReadString('\n')

		if err != nil {
			return err
		}

		input = strings.TrimSpace(input)

		if input == "" {
			input = "127.0.0.1:4125"
		}

		config.ServerAddress = input

		fmt.Println()
	}

	config.AutoUpdate = true
	config.UpdateInterval = DefaultUpdateInterval

	m.config = config

	if err := m.saveConfig(); err != nil {
		return err
	}

	fmt.Println(
		"Configuration saved.",
	)

	return nil
}

func promptNumber(
	reader *bufio.Reader,
	prompt string,
	min int,
	max int,
) (int, error) {

	for {
		fmt.Print(prompt)

		input, err := reader.ReadString('\n')

		if err != nil {
			return 0, err
		}

		input = strings.TrimSpace(input)

		if input == "" {
			return min, nil
		}

		value, err := strconv.Atoi(input)

		if err == nil &&
			value >= min &&
			value <= max {

			return value, nil
		}

		fmt.Printf(
			"Please enter a number between %d and %d.\n",
			min,
			max,
		)
	}
}

func (m *Manager) saveConfig() error {
	data, err := json.MarshalIndent(
		m.config,
		"",
		"    ",
	)

	if err != nil {
		return err
	}

	tmp := m.configPath + ".tmp"

	if err := os.WriteFile(
		tmp,
		data,
		0644,
	); err != nil {
		return err
	}

	return os.Rename(
		tmp,
		m.configPath,
	)
}

func (m *Manager) loadConfig() error {
	data, err := os.ReadFile(
		m.configPath,
	)

	if err != nil {
		return err
	}

	if err := json.Unmarshal(
		data,
		&m.config,
	); err != nil {
		return err
	}

	if m.config.UpdateInterval <= 0 {
		m.config.UpdateInterval =
			DefaultUpdateInterval
	}

	return nil
}

// ============================================================
// GitHub / update information
// ============================================================

func latestURL() string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/latest.json",
		GitHubOwner,
		GitHubRepo,
		BuildBranch,
	)
}

func binaryURL(
	version string,
	platform string,
	binary string,
) string {

	extension := ""

	if runtime.GOOS == "windows" {
		extension = ".exe"
	}

	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/%s/%s/%s%s",
		GitHubOwner,
		GitHubRepo,
		BuildBranch,
		version,
		platform,
		binary,
		extension,
	)
}

func checksumsURL(
	version string,
	platform string,
) string {

	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/%s/%s/SHA256SUMS",
		GitHubOwner,
		GitHubRepo,
		BuildBranch,
		version,
		platform,
	)
}

func fetchLatest() (*LatestBuild, error) {
	var latest LatestBuild

	if err := httpJSON(
		latestURL(),
		&latest,
	); err != nil {
		return nil, err
	}

	if latest.Version == "" {
		return nil, errors.New(
			"latest.json did not contain a version",
		)
	}

	return &latest, nil
}

func httpJSON(
	url string,
	output any,
) error {

	req, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)

	if err != nil {
		return err
	}

	req.Header.Set(
		"User-Agent",
		"ClearLink",
	)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"HTTP %s",
			resp.Status,
		)
	}

	return json.NewDecoder(
		resp.Body,
	).Decode(output)
}

func platformName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows-amd64"

	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "linux-amd64"

		case "arm64":
			return "linux-arm64"
		}
	}

	return ""
}

// ============================================================
// Installation
// ============================================================

func (m *Manager) ensureInstalled() error {
	latest, err := fetchLatest()

	if err != nil {
		return fmt.Errorf(
			"checking latest build: %w",
			err,
		)
	}

	fmt.Println(
		"Latest successful build:",
		latest.Version,
	)

	if latest.Version == m.config.Version &&
		m.hasInstalledBinary() {

		fmt.Println(
			"Already running latest version.",
		)

		return nil
	}

	fmt.Println()
	fmt.Println("Update available.")
	fmt.Println()
	fmt.Println(
		"Current:",
		displayVersion(m.config.Version),
	)
	fmt.Println(
		"Latest:",
		displayVersion(latest.Version),
	)
	fmt.Println()

	return m.installVersion(
		latest.Version,
	)
}

func (m *Manager) installVersion(
	version string,
) error {

	platform := platformName()

	if platform == "" {
		return errors.New(
			"unsupported platform",
		)
	}

	fmt.Println(
		"Platform:",
		platform,
	)

	// Download every binary that this configuration
	// can potentially need.
	binaries := m.binariesForLink()

	tempDir, err := os.MkdirTemp(
		"",
		"clearlink-update-*",
	)

	if err != nil {
		return err
	}

	defer os.RemoveAll(tempDir)

	versionDir := filepath.Join(
		m.versionDir,
		version,
	)

	if err := os.MkdirAll(
		versionDir,
		0755,
	); err != nil {
		return err
	}

	for _, binary := range binaries {
		fmt.Println()
		fmt.Println(
			"Downloading",
			binary,
			"...",
		)

		tempPath := filepath.Join(
			tempDir,
			binary,
		)

		url := binaryURL(
			version,
			platform,
			binary,
		)

		if err := downloadFile(
			url,
			tempPath,
		); err != nil {
			return fmt.Errorf(
				"downloading %s: %w",
				binary,
				err,
			)
		}

		fmt.Println(
			"Downloaded",
			binary,
		)

		if err := installFile(
			tempPath,
			filepath.Join(
				versionDir,
				executableName(binary),
			),
		); err != nil {
			return err
		}
	}

	// Verify all downloaded binaries.
	checksumURL := checksumsURL(
		version,
		platform,
	)

	checksumPath := filepath.Join(
		tempDir,
		"SHA256SUMS",
	)

	if err := downloadFile(
		checksumURL,
		checksumPath,
	); err != nil {
		return fmt.Errorf(
			"downloading checksums: %w",
			err,
		)
	}

	if err := verifyChecksums(
		checksumPath,
		versionDir,
		binaries,
	); err != nil {
		return err
	}

	// If ClearLink is already running, stop it before
	// replacing current/.
	if m.child != nil {
		if err := m.stopChild(); err != nil {
			return err
		}
	}

	if err := replaceDirectory(
		versionDir,
		m.currentDir,
	); err != nil {
		return err
	}

	m.config.Version = version

	if err := m.saveConfig(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(
		"✓ ClearLink",
		displayVersion(version),
		"installed.",
	)

	return nil
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}

	return name
}

func installFile(
	source string,
	destination string,
) error {

	if err := os.MkdirAll(
		filepath.Dir(destination),
		0755,
	); err != nil {
		return err
	}

	src, err := os.Open(source)

	if err != nil {
		return err
	}

	defer src.Close()

	dst, err := os.Create(destination)

	if err != nil {
		return err
	}

	if _, err := io.Copy(
		dst,
		src,
	); err != nil {
		dst.Close()
		return err
	}

	if err := dst.Close(); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		return os.Chmod(
			destination,
			0755,
		)
	}

	return nil
}

func verifyChecksums(
	checksumFile string,
	directory string,
	requiredFiles []string,
) error {
	data, err := os.ReadFile(checksumFile)
	if err != nil {
		return err
	}

	checksums := make(map[string]string)

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		fields := strings.Fields(line)

		if len(fields) < 2 {
			return fmt.Errorf(
				"invalid checksum line: %q",
				line,
			)
		}

		hash := strings.ToLower(fields[0])

		filename := strings.TrimPrefix(
			fields[len(fields)-1],
			"*",
		)

		// Handle checksums generated as:
		//
		// hash  broadcast
		//
		// as well as:
		//
		// hash  output/broadcast
		//
		filename = filepath.Base(filename)

		checksums[filename] = hash
	}

	for _, filename := range requiredFiles {
		filename = filepath.Base(filename)

		expected, ok := checksums[filename]
		if !ok {
			return fmt.Errorf(
				"no checksum found for %s",
				filename,
			)
		}

		path := filepath.Join(
			directory,
			filename,
		)

		actual, err := sha256File(path)
		if err != nil {
			return fmt.Errorf(
				"verifying %s: %w",
				filename,
				err,
			)
		}

		if !strings.EqualFold(
			actual,
			expected,
		) {
			return fmt.Errorf(
				"SHA-256 mismatch for %s\nexpected: %s\nactual:   %s",
				filename,
				expected,
				actual,
			)
		}

		fmt.Println(
			"✓ Verified",
			filename,
		)
	}

	return nil
}

func sha256File(
	path string,
) (string, error) {

	file, err := os.Open(path)

	if err != nil {
		return "", err
	}

	defer file.Close()

	hash := sha256.New()

	if _, err := io.Copy(
		hash,
		file,
	); err != nil {
		return "", err
	}

	return hex.EncodeToString(
		hash.Sum(nil),
	), nil
}

// ============================================================
// Runtime
// ============================================================

func (m *Manager) run() error {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)

	defer cancel()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("         ClearLink is running")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println(
		"Link type:",
		m.config.LinkType,
	)
	fmt.Println(
		"Version:",
		displayVersion(m.config.Version),
	)
	fmt.Println()

	if err := m.startChild(); err != nil {
		return err
	}

	ticker := time.NewTicker(
		m.config.UpdateInterval,
	)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			fmt.Println(
				"Shutting down ClearLink...",
			)

			return m.stopChild()

		case <-ticker.C:
			if m.config.AutoUpdate {
				if err := m.checkForUpdate(); err != nil {
					fmt.Println(
						"Update check failed:",
						err,
					)
				}
			}
		}
	}
}

func (m *Manager) checkForUpdate() error {
	fmt.Println()
	fmt.Println(
		"Checking for ClearLink updates...",
	)

	latest, err := fetchLatest()

	if err != nil {
		return err
	}

	if latest.Version == m.config.Version {
		fmt.Println(
			"✓ Already up to date.",
		)

		return nil
	}

	fmt.Println()
	fmt.Println(
		"========================================",
	)
	fmt.Println(
		"          UPDATE AVAILABLE",
	)
	fmt.Println(
		"========================================",
	)
	fmt.Println()

	fmt.Println(
		"Current:",
		displayVersion(m.config.Version),
	)

	fmt.Println(
		"Latest:",
		displayVersion(latest.Version),
	)

	fmt.Println()

	if err := m.installVersion(
		latest.Version,
	); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(
		"Starting updated ClearLink...",
	)

	if err := m.startChild(); err != nil {
		return err
	}

	fmt.Println(
		"✓ Update complete.",
	)

	return nil
}

func (m *Manager) startChild() error {
	if m.child != nil {
		return nil
	}

	binaries := m.binariesForLink()

	if len(binaries) == 0 {
		return errors.New(
			"no executable selected",
		)
	}

	// Single-process configuration.
	if len(binaries) == 1 {
		return m.startSingle(
			binaries[0],
		)
	}

	// Broadcast + Listen requires two child processes.
	// See note below.
	return errors.New(
		"broadcast-listen mode needs multi-process support",
	)
}

func (m *Manager) startSingle(
	name string,
) error {

	path := filepath.Join(
		m.currentDir,
		executableName(name),
	)

	if !fileExists(path) {
		return fmt.Errorf(
			"binary not found: %s",
			path,
		)
	}

	args := m.argumentsFor(name)

	fmt.Println(
		"Starting:",
		name,
	)

	cmd := exec.Command(
		path,
		args...,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return err
	}

	m.child = cmd

	fmt.Println(
		"Started with PID",
		cmd.Process.Pid,
	)

	return nil
}

func (m *Manager) stopChild() error {
	if m.child == nil ||
		m.child.Process == nil {

		m.child = nil

		return nil
	}

	fmt.Println(
		"Stopping ClearLink...",
	)

	process := m.child.Process

	if runtime.GOOS == "windows" {
		_ = process.Kill()
	} else {
		_ = process.Signal(
			syscall.SIGTERM,
		)
	}

	done := make(chan error, 1)

	go func() {
		done <- m.child.Wait()
	}()

	select {
	case <-done:
		m.child = nil

	case <-time.After(ShutdownTimeout):
		fmt.Println(
			"ClearLink did not exit gracefully.",
		)

		_ = process.Kill()

		<-done

		m.child = nil
	}

	fmt.Println(
		"ClearLink stopped.",
	)

	return nil
}

func (m *Manager) binariesForLink() []string {
	switch m.config.LinkType {
	case "broadcast":
		return []string{"broadcast"}

	case "listen":
		return []string{"listen"}

	case "server":
		return []string{"server"}

	case "broadcast-listen":
		return []string{
			"broadcast",
			"listen",
		}
	}

	return nil
}

func (m *Manager) argumentsFor(
	name string,
) []string {

	var args []string

	switch name {
	case "broadcast":

		if m.config.ServerAddress != "" {
			args = append(
				args,
				"--server",
				m.config.ServerAddress,
			)
		}

		if m.config.Frequency != 0 {
			args = append(
				args,
				"--frequency",
				strconv.FormatInt(
					m.config.Frequency,
					10,
				),
			)
		}

	case "listen":

		if m.config.ServerAddress != "" {
			args = append(
				args,
				"--server",
				m.config.ServerAddress,
			)
		}

		if m.config.Frequency != 0 {
			args = append(
				args,
				"--frequency",
				strconv.FormatInt(
					m.config.Frequency,
					10,
				),
			)
		}

		args = append(
			args,
			"--device",
			strconv.Itoa(
				m.config.Device,
			),
		)

	case "server":
		// Add server arguments here.
	}

	return args
}

// ============================================================
// Filesystem
// ============================================================

func (m *Manager) hasInstalledBinary() bool {
	for _, binary := range m.binariesForLink() {
		if !fileExists(
			filepath.Join(
				m.currentDir,
				executableName(binary),
			),
		) {
			return false
		}
	}

	return true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil &&
		!info.IsDir()
}

func replaceDirectory(
	source string,
	destination string,
) error {

	backup := destination + ".old"

	_ = os.RemoveAll(backup)

	if _, err := os.Stat(
		destination,
	); err == nil {

		if err := os.Rename(
			destination,
			backup,
		); err != nil {
			return err
		}
	}

	if err := os.Rename(
		source,
		destination,
	); err != nil {

		_ = os.Rename(
			backup,
			destination,
		)

		return err
	}

	_ = os.RemoveAll(backup)

	return nil
}

// ============================================================
// HTTP
// ============================================================

func downloadFile(
	url string,
	destination string,
) error {

	req, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)

	if err != nil {
		return err
	}

	req.Header.Set(
		"User-Agent",
		"ClearLink",
	)

	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"download failed: %s",
			resp.Status,
		)
	}

	file, err := os.Create(
		destination,
	)

	if err != nil {
		return err
	}

	defer file.Close()

	_, err = io.Copy(
		file,
		resp.Body,
	)

	return err
}

func displayVersion(version string) string {
	if version == "" {
		return "not installed"
	}

	if len(version) > 8 {
		return version[:8]
	}

	return version
}

func fatal(message string) {
	fmt.Fprintln(
		os.Stderr,
		"ERROR:",
		message,
	)

	os.Exit(1)
}