package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"time"
)

type config struct {
	LiveRetryTimeout      int            `json:"live_retry_timeout"`
	Lang                  string         `json:"preferred_language"`
	CheckUpdate           bool           `json:"check_updates"`
	SaveLogs              bool           `json:"save_logs"`
	LogLocation           string         `json:"log_location"`
	CustomPlaybackOptions []command      `json:"custom_playback_options"`
	MultiCommand          []multiCommand `json:"multi_commands"`
	HorizontalLayout      bool           `json:"horizontal_layout"`
	Theme                 theme          `json:"theme"`
	TreeRatio             int            `json:"tree_ratio"`
	OutputRatio           int            `json:"output_ratio"`
	TerminalWrap          bool           `json:"terminal_wrap"`
}

type theme struct {
	BackgroundColor     string `json:"background_color"`
	BorderColor         string `json:"border_color"`
	CategoryNodeColor   string `json:"category_node_color"`
	FolderNodeColor     string `json:"folder_node_color"`
	ItemNodeColor       string `json:"item_node_color"`
	ActionNodeColor     string `json:"action_node_color"`
	LoadingColor        string `json:"loading_color"`
	LiveColor           string `json:"live_color"`
	UpdateColor         string `json:"update_color"`
	NoContentColor      string `json:"no_content_color"`
	InfoColor           string `json:"info_color"`
	ErrorColor          string `json:"error_color"`
	TerminalAccentColor string `json:"terminal_accent_color"`
	TerminalTextColor   string `json:"terminal_text_color"`
	MultiCommandColor   string `json:"multi_command_color"`
}

func loadConfig() (config, error) {
	var cfg config
	path, err := getConfigPath()
	if err != nil {
		return cfg, err
	}

	if _, err = os.Stat(path + "config.json"); os.IsNotExist(err) {
		cfg.LiveRetryTimeout = 60
		cfg.Lang = "en"
		cfg.CheckUpdate = true
		cfg.SaveLogs = true
		cfg.TreeRatio = 1
		cfg.OutputRatio = 1
		cfg.TerminalWrap = true
		err = cfg.save()
		return cfg, err
	}

	var data []byte
	data, err = ioutil.ReadFile(path + "config.json")
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	if cfg.TreeRatio < 1 {
		cfg.TreeRatio = 1
	}
	if cfg.OutputRatio < 1 {
		cfg.OutputRatio = 1
	}
	cfg.Theme.apply()
	return cfg, err
}

func getConfigPath() (string, error) {
	path, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path += string(os.PathSeparator) + "f1viewer" + string(os.PathSeparator)

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
	}
	return path, err
}

func getLogPath(cfg config) (string, error) {
	var path string
	if cfg.LogLocation == "" {
		// windows, macos
		switch runtime.GOOS {
		case "windows", "darwin":
			configPath, err := getConfigPath()
			if err != nil {
				return "", err
			}
			path = configPath + string(os.PathSeparator) + "logs" + string(os.PathSeparator)
		default:
			// linux, etc.
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			path = home + "/.local/share/f1viewer/"
		}
	} else {
		path = cfg.LogLocation
	}

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
	}
	return path, err
}

func (cfg config) save() error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(&cfg, "", "\t")
	if err != nil {
		return fmt.Errorf("error marshaling config: %v", err)
	}

	err = ioutil.WriteFile(path+"config.json", data, os.ModePerm)
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}
	return nil
}

func configureLogging(cfg config) (*os.File, error) {
	if !cfg.SaveLogs {
		log.SetOutput(ioutil.Discard)
		return nil, nil
	}
	logPath, err := getLogPath(cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not get log path: %w", err)
	}
	completePath := fmt.Sprint(logPath, time.Now().Format("2006-01-02"), ".log")
	logFile, err := os.OpenFile(completePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("Could not open log file: %w", err)
	}
	log.SetOutput(logFile)
	return logFile, nil
}
