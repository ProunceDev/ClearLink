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

	"gopkg.in/ini.v1"
)

const (
	GitHubOwner = "ProunceDev"
	GitHubRepo  = "ClearLink"
	BuildBranch = "builds"

	DefaultUpdateInterval = 15 * time.Second
	ShutdownTimeout       = 10 * time.Second
	RestartDelay          = 2 * time.Second
)

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
	ansiBold   = "\033[1m"
)

type Config struct {
	LinkType string `json:"link_type"`

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

type childExitEvent struct {
	name string
	err  error
}

type Manager struct {
	config Config

	configPath     string
	linkConfigPath string
	baseDir        string
	currentDir     string
	versionDir     string

	child           *exec.Cmd
	childName       string
	stoppingChild   bool
	childExitCh     chan childExitEvent
	lastUpdateError string
}

func main() {
	printBanner("ClearLink Manager")

	if runtime.GOOS != "linux" &&
		runtime.GOOS != "windows" {
		fatal(
			"unsupported operating system: "+
				runtime.GOOS,
		)
	}

	if runtime.GOOS == "linux" &&
		runtime.GOARCH != "amd64" &&
		runtime.GOARCH != "arm64" {
		fatal(
			"unsupported Linux architecture: "+
				runtime.GOARCH,
		)
	}

	if runtime.GOOS == "windows" &&
		runtime.GOARCH != "amd64" {
		fatal(
			"unsupported Windows architecture: "+
				runtime.GOARCH,
		)
	}

	manager, err := newManager()
	if err != nil {
		fatal(err.Error())
	}

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

		info("Configuration loaded")
		info("Link type: %s", manager.config.LinkType)
		info("Installed version: %s", displayVersion(manager.config.Version))

		if !fileExists(manager.linkConfigPath) {
			warn("%s is missing; entering setup", manager.linkConfigPath)
			if err := manager.rebuildLinkConfig(); err != nil {
				fatal(err.Error())
			}
		}
	}

	info("Installer base directory: %s", manager.baseDir)
	info("Checking ClearLink installation")

	if err := manager.ensureInstalled(); err != nil {
		fatal(err.Error())
	}

	if err := manager.run(); err != nil {
		fatal(err.Error())
	}
}

func newManager() (*Manager, error) {
	baseDir, err := executableDir()
	if err != nil {
		return nil, err
	}

	return &Manager{
		baseDir: baseDir,
		childExitCh: make(
			chan childExitEvent,
			4,
		),

		configPath: filepath.Join(
			baseDir,
			"manager.json",
		),

		linkConfigPath: filepath.Join(
			baseDir,
			"config.ini",
		),

		currentDir: filepath.Join(
			baseDir,
			"current",
		),

		versionDir: filepath.Join(
			baseDir,
			"versions",
		),
	}, nil
}

func executableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	realPath, err := filepath.EvalSymlinks(exePath)
	if err == nil {
		exePath = realPath
	}

	return filepath.Dir(exePath), nil
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

	printBanner("First-Time Setup")
	info("This appears to be your first run")
	info("A local config.ini will be written next to this executable")

	cfg := Config{
		AutoUpdate:     true,
		UpdateInterval: DefaultUpdateInterval,
	}

	fmt.Println()
	info("What type of link are you setting up?")
	info("  1) Broadcast")
	info("  2) Listen")
	info("  3) Server")
	fmt.Println()

	choice, err := promptNumber(
		reader,
		"Select [1-3]: ",
		1,
		3,
	)
	if err != nil {
		return err
	}

	switch choice {
	case 1:
		cfg.LinkType = "broadcast"
	case 2:
		cfg.LinkType = "listen"
	case 3:
		cfg.LinkType = "server"
	}

	if err := m.configureLinkConfig(reader, cfg.LinkType); err != nil {
		return err
	}

	m.config = cfg

	if err := m.saveConfig(); err != nil {
		return err
	}

	success("Setup complete")
	return nil
}

func (m *Manager) rebuildLinkConfig() error {
	reader := bufio.NewReader(os.Stdin)
	return m.configureLinkConfig(reader, m.config.LinkType)
}

func (m *Manager) configureLinkConfig(reader *bufio.Reader, linkType string) error {
	cfg := ini.Empty()

	switch linkType {
	case "server":
		if err := configureServerSection(reader, cfg); err != nil {
			return err
		}
	case "broadcast":
		if err := configureBroadcastSection(reader, cfg); err != nil {
			return err
		}
	case "listen":
		if err := configureListenSection(reader, cfg); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported link type: %s", linkType)
	}

	tmp := m.linkConfigPath + ".tmp"
	if err := cfg.SaveTo(tmp); err != nil {
		return err
	}

	if err := os.Rename(tmp, m.linkConfigPath); err != nil {
		return err
	}

	success("Wrote link config: %s", m.linkConfigPath)
	return nil
}

func configureServerSection(reader *bufio.Reader, cfg *ini.File) error {
	printBanner("Server Settings")

	serverPort, err := promptIntWithDefault(reader, "ServerPort", 4125)
	if err != nil {
		return err
	}

	webPort, err := promptIntWithDefault(reader, "WebPort", 44325)
	if err != nil {
		return err
	}

	authKey, err := promptStringWithDefault(reader, "AuthKey", "CHANGEME")
	if err != nil {
		return err
	}

	adminUser, err := promptStringWithDefault(reader, "Admin Username", "admin")
	if err != nil {
		return err
	}

	adminPassword, err := promptRequiredString(reader, "Admin Password")
	if err != nil {
		return err
	}

	section := cfg.Section("server")
	section.Key("Port").SetValue(strconv.Itoa(serverPort))
	section.Key("WebPort").SetValue(strconv.Itoa(webPort))
	section.Key("AuthKey").SetValue(authKey)
	section.Key("AdminUsername").SetValue(adminUser)
	section.Key("AdminPassword").SetValue(adminPassword)

	return nil
}

func configureBroadcastSection(reader *bufio.Reader, cfg *ini.File) error {
	printBanner("Broadcast Settings")

	authKey, err := promptStringWithDefault(reader, "AuthKey", "CHANGEME")
	if err != nil {
		return err
	}

	serverPort, err := promptIntWithDefault(reader, "ServerPort", 4125)
	if err != nil {
		return err
	}

	serverAddr, err := promptRequiredString(reader, "ServerAddr")
	if err != nil {
		return err
	}

	nodeName, err := promptRequiredString(reader, "NodeName")
	if err != nil {
		return err
	}

	outputType, err := promptChoice(reader, "Type (DISCORD or RADIO)", []string{"DISCORD", "RADIO"}, "DISCORD")
	if err != nil {
		return err
	}

	section := cfg.Section("broadcast")
	section.Key("AuthKey").SetValue(authKey)
	section.Key("ServerPort").SetValue(strconv.Itoa(serverPort))
	section.Key("ServerAddr").SetValue(serverAddr)
	section.Key("NodeName").SetValue(nodeName)
	section.Key("Type").SetValue(outputType)

	if outputType == "RADIO" {
		pttPin, err := promptIntWithDefault(reader, "Ptt_Pin", 4)
		if err != nil {
			return err
		}

		section.Key("Ptt_Pin").SetValue(strconv.Itoa(pttPin))
	}

	if outputType == "DISCORD" {
		botToken, err := promptRequiredString(reader, "BotToken")
		if err != nil {
			return err
		}

		guildID, err := promptRequiredString(reader, "GuildID")
		if err != nil {
			return err
		}

		voiceChannelID, err := promptRequiredString(reader, "VoiceChannelID")
		if err != nil {
			return err
		}

		section.Key("BotToken").SetValue(botToken)
		section.Key("GuildID").SetValue(guildID)
		section.Key("VoiceChannelID").SetValue(voiceChannelID)
	}

	return nil
}

func configureListenSection(reader *bufio.Reader, cfg *ini.File) error {
	printBanner("Listen Settings")

	authKey, err := promptStringWithDefault(reader, "AuthKey", "CHANGEME")
	if err != nil {
		return err
	}

	serverPort, err := promptIntWithDefault(reader, "ServerPort", 4125)
	if err != nil {
		return err
	}

	serverAddr, err := promptRequiredString(reader, "ServerAddr")
	if err != nil {
		return err
	}

	nodeName, err := promptRequiredString(reader, "NodeName")
	if err != nil {
		return err
	}

	frequencyHz, err := promptFrequencyHz(reader, "Frequency (MHz, e.g. 146.520)")
	if err != nil {
		return err
	}

	section := cfg.Section("listen")
	section.Key("AuthKey").SetValue(authKey)
	section.Key("ServerPort").SetValue(strconv.Itoa(serverPort))
	section.Key("ServerAddr").SetValue(serverAddr)
	section.Key("NodeName").SetValue(nodeName)
	section.Key("Frequency").SetValue(strconv.Itoa(frequencyHz))

	return nil
}

func promptNumber(reader *bufio.Reader, prompt string, min int, max int) (int, error) {
	for {
		text, err := promptRaw(reader, prompt)
		if err != nil {
			return 0, err
		}

		if text == "" {
			return min, nil
		}

		value, err := strconv.Atoi(text)
		if err == nil && value >= min && value <= max {
			return value, nil
		}

		warn("Please enter a number between %d and %d", min, max)
	}
}

func promptIntWithDefault(reader *bufio.Reader, key string, defaultValue int) (int, error) {
	prompt := fmt.Sprintf("%s [%d]: ", key, defaultValue)

	for {
		text, err := promptRaw(reader, prompt)
		if err != nil {
			return 0, err
		}

		if text == "" {
			return defaultValue, nil
		}

		value, err := strconv.Atoi(text)
		if err == nil {
			return value, nil
		}

		warn("Please enter a valid integer")
	}
}

func promptStringWithDefault(reader *bufio.Reader, key string, defaultValue string) (string, error) {
	prompt := fmt.Sprintf("%s [%s]: ", key, defaultValue)
	text, err := promptRaw(reader, prompt)
	if err != nil {
		return "", err
	}

	if text == "" {
		return defaultValue, nil
	}

	return text, nil
}

func promptRequiredString(reader *bufio.Reader, key string) (string, error) {
	prompt := fmt.Sprintf("%s: ", key)

	for {
		text, err := promptRaw(reader, prompt)
		if err != nil {
			return "", err
		}

		if text != "" {
			return text, nil
		}

		warn("%s is required", key)
	}
}

func promptChoice(reader *bufio.Reader, key string, choices []string, defaultChoice string) (string, error) {
	prompt := fmt.Sprintf("%s [%s]: ", key, defaultChoice)

	valid := map[string]struct{}{}
	for _, choice := range choices {
		valid[strings.ToUpper(choice)] = struct{}{}
	}

	for {
		text, err := promptRaw(reader, prompt)
		if err != nil {
			return "", err
		}

		if text == "" {
			return defaultChoice, nil
		}

		text = strings.ToUpper(text)
		if _, ok := valid[text]; ok {
			return text, nil
		}

		warn("Valid options: %s", strings.Join(choices, ", "))
	}
}

func promptFrequencyHz(reader *bufio.Reader, label string) (int, error) {
	for {
		text, err := promptRaw(reader, label+": ")
		if err != nil {
			return 0, err
		}

		value, err := parseFrequency(text)
		if err == nil {
			return value, nil
		}

		warn("Enter a value like 146.520")
	}
}

func parseFrequency(input string) (int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, errors.New("empty frequency")
	}

	if strings.Contains(input, ".") {
		mhz, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return 0, err
		}

		hz := int(mhz*1_000_000 + 0.5)
		if hz <= 0 {
			return 0, errors.New("frequency must be positive")
		}

		return hz, nil
	}

	hz, err := strconv.Atoi(input)
	if err != nil {
		return 0, err
	}

	if hz <= 0 {
		return 0, errors.New("frequency must be positive")
	}

	return hz, nil
}

func promptRaw(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(colorize(ansiCyan+ansiBold, prompt))
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
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

func latestURL() string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/latest.json",
		GitHubOwner,
		GitHubRepo,
		BuildBranch,
	)
}

func binaryURL(version string, platform string, binary string) string {
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

func checksumsURL(version string, platform string) string {
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

	if err := httpJSON(latestURL(), &latest); err != nil {
		return nil, err
	}

	if latest.Version == "" {
		return nil, errors.New("latest.json did not contain a version")
	}

	return &latest, nil
}

func httpJSON(url string, output any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "ClearLink")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(output)
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

func (m *Manager) ensureInstalled() error {
	latest, err := fetchLatest()
	if err != nil {
		return fmt.Errorf("checking latest build: %w", err)
	}

	if latest.Version == m.config.Version && m.hasInstalledBinary() {
		success("Already running latest version: %s", displayVersion(latest.Version))
		return nil
	}

	if m.config.Version == "" || !m.hasInstalledBinary() {
		info("Installing %s", displayVersion(latest.Version))
	} else {
		warn("Update available: %s -> %s", displayVersion(m.config.Version), displayVersion(latest.Version))
	}

	return m.installVersion(latest.Version)
}

func (m *Manager) installVersion(version string) error {
	platform := platformName()
	if platform == "" {
		return errors.New("unsupported platform")
	}

	info("Target platform: %s", platform)

	binaries := m.binariesForLink()
	tempDir, err := os.MkdirTemp("", "clearlink-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	versionDir := filepath.Join(m.versionDir, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return err
	}

	for _, binary := range binaries {
		info("Downloading %s", binary)

		tempPath := filepath.Join(tempDir, binary)
		url := binaryURL(version, platform, binary)

		if err := downloadFile(url, tempPath); err != nil {
			return fmt.Errorf("downloading %s: %w", binary, err)
		}

		if err := installFile(tempPath, filepath.Join(versionDir, executableName(binary))); err != nil {
			return err
		}

		success("Downloaded %s", binary)
	}

	checksumURL := checksumsURL(version, platform)
	checksumPath := filepath.Join(tempDir, "SHA256SUMS")

	if err := downloadFile(checksumURL, checksumPath); err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	if err := verifyChecksums(checksumPath, versionDir, binaries); err != nil {
		return err
	}

	if m.child != nil {
		if err := m.stopChild(); err != nil {
			return err
		}
	}

	if err := replaceDirectory(versionDir, m.currentDir); err != nil {
		return err
	}

	m.config.Version = version
	if err := m.saveConfig(); err != nil {
		return err
	}

	success("Installed ClearLink %s", displayVersion(version))
	return nil
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}

	return name
}

func installFile(source string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
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

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}

	if err := dst.Close(); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		return os.Chmod(destination, 0755)
	}

	return nil
}

func verifyChecksums(checksumFile string, directory string, requiredFiles []string) error {
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
			return fmt.Errorf("invalid checksum line: %q", line)
		}

		hash := strings.ToLower(fields[0])
		filename := strings.TrimPrefix(fields[len(fields)-1], "*")
		filename = filepath.Base(filename)
		checksums[filename] = hash
	}

	for _, filename := range requiredFiles {
		filename = filepath.Base(filename)

		expected, ok := checksums[filename]
		if !ok {
			return fmt.Errorf("no checksum found for %s", filename)
		}

		path := filepath.Join(directory, filename)
		actual, err := sha256File(path)
		if err != nil {
			return fmt.Errorf("verifying %s: %w", filename, err)
		}

		if !strings.EqualFold(actual, expected) {
			return fmt.Errorf("SHA-256 mismatch for %s\nexpected: %s\nactual:   %s", filename, expected, actual)
		}

		success("Verified %s", filename)
	}

	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (m *Manager) run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	printBanner("ClearLink Running")
	info("Link type: %s", m.config.LinkType)
	info("Version: %s", displayVersion(m.config.Version))
	info("Working directory: %s", m.baseDir)

	if err := m.startChild(); err != nil {
		return err
	}

	ticker := time.NewTicker(m.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			warn("Shutdown requested")
			return m.stopChild()

		case exit := <-m.childExitCh:
			m.child = nil
			m.childName = ""

			if m.stoppingChild {
				m.stoppingChild = false
				continue
			}

			if exit.err != nil {
				warn("%s exited unexpectedly: %v", exit.name, exit.err)
			} else {
				warn("%s exited unexpectedly", exit.name)
			}

			warn("Restarting in %s", RestartDelay)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(RestartDelay):
			}

			if err := m.startChild(); err != nil {
				return err
			}

		case <-ticker.C:
			if !m.config.AutoUpdate {
				continue
			}

			updated, err := m.checkForUpdate()
			if err != nil {
				if err.Error() != m.lastUpdateError {
					warn("Update check failed: %v", err)
					m.lastUpdateError = err.Error()
				}
				continue
			}

			m.lastUpdateError = ""
			if updated {
				success("Update complete")
			}
		}
	}
}

func (m *Manager) checkForUpdate() (bool, error) {
	latest, err := fetchLatest()
	if err != nil {
		return false, err
	}

	if latest.Version == m.config.Version {
		return false, nil
	}

	warn("Update available: %s -> %s", displayVersion(m.config.Version), displayVersion(latest.Version))

	if err := m.installVersion(latest.Version); err != nil {
		return false, err
	}

	if err := m.startChild(); err != nil {
		return false, err
	}

	return true, nil
}

func (m *Manager) startChild() error {
	if m.child != nil {
		return nil
	}

	binaries := m.binariesForLink()
	if len(binaries) == 0 {
		return errors.New("no executable selected")
	}

	if len(binaries) == 1 {
		return m.startSingle(binaries[0])
	}

	return errors.New("broadcast-listen mode needs multi-process support")
}

func (m *Manager) startSingle(name string) error {
	path := filepath.Join(m.currentDir, executableName(name))

	if !fileExists(path) {
		return fmt.Errorf("binary not found: %s", path)
	}

	info("Starting %s", name)

	cmd := exec.Command(path)
	cmd.Dir = m.baseDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return err
	}

	m.child = cmd
	m.childName = name
	m.stoppingChild = false

	go func(name string, cmd *exec.Cmd) {
		m.childExitCh <- childExitEvent{
			name: name,
			err:  cmd.Wait(),
		}
	}(name, cmd)

	success("Started PID %d", cmd.Process.Pid)
	return nil
}

func (m *Manager) stopChild() error {
	if m.child == nil || m.child.Process == nil {
		m.child = nil
		m.childName = ""
		m.stoppingChild = false
		return nil
	}

	warn("Stopping ClearLink process")
	process := m.child.Process
	m.stoppingChild = true

	if runtime.GOOS == "windows" {
		_ = process.Kill()
	} else {
		_ = process.Signal(syscall.SIGTERM)
	}

	select {
	case <-m.childExitCh:
		m.child = nil
		m.childName = ""
		m.stoppingChild = false
	case <-time.After(ShutdownTimeout):
		warn("Process did not exit gracefully; force killing")
		_ = process.Kill()

		select {
		case <-m.childExitCh:
			m.child = nil
			m.childName = ""
			m.stoppingChild = false
		case <-time.After(ShutdownTimeout):
			m.child = nil
			m.childName = ""
			m.stoppingChild = false
			return errors.New("process did not terminate after force kill")
		}
	}

	success("Process stopped")
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
		return []string{"broadcast", "listen"}
	}

	return nil
}

func (m *Manager) hasInstalledBinary() bool {
	for _, binary := range m.binariesForLink() {
		if !fileExists(filepath.Join(m.currentDir, executableName(binary))) {
			return false
		}
	}

	return true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func replaceDirectory(source string, destination string) error {
	backup := destination + ".old"
	_ = os.RemoveAll(backup)

	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	}

	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}

	_ = os.RemoveAll(backup)
	return nil
}

func downloadFile(url string, destination string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "ClearLink")

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
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

func supportsColor() bool {
	if runtime.GOOS == "windows" {
		return true
	}

	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}

func colorize(color string, message string) string {
	if !supportsColor() {
		return message
	}

	return color + message + ansiReset
}

func printBanner(title string) {
	line := strings.Repeat("=", 40)
	fmt.Println()
	fmt.Println(colorize(ansiCyan, line))
	fmt.Println(colorize(ansiCyan+ansiBold, center(title, 40)))
	fmt.Println(colorize(ansiCyan, line))
	fmt.Println()
}

func center(text string, width int) string {
	if len(text) >= width {
		return text
	}

	left := (width - len(text)) / 2
	right := width - len(text) - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

func info(format string, args ...any) {
	fmt.Println(colorize(ansiBlue, "[INFO] ") + fmt.Sprintf(format, args...))
}

func success(format string, args ...any) {
	fmt.Println(colorize(ansiGreen, "[ OK ] ") + fmt.Sprintf(format, args...))
}

func warn(format string, args ...any) {
	fmt.Println(colorize(ansiYellow, "[WARN] ") + fmt.Sprintf(format, args...))
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, colorize(ansiRed+ansiBold, "[FAIL] ")+message)
	os.Exit(1)
}