package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	"github.com/pkg/errors"
)

// getConfigPath populates the configPath global variable, returning errors in the process,
// and, if any, leaves configPath unchanged.
func getConfigPath() error {
	configPathSuffix := "/drivesync/config.json"
	usr, err := user.Current()
	if err != nil {
		return err
	}
	configRoots := []string{
		"",
		"/etc",
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		configRoots[0] = v
	} else {
		configRoots[0] = usr.HomeDir + "/.config"
	}
	var found bool
	for _, v := range configRoots {
		path := filepath.Clean(v) + configPathSuffix
		fi, err := os.Stat(path)
		if err == nil && !fi.IsDir() {
			configPath = path
			found = true
			break
		}
	}
	if !found {
		// we'll need to create the config file
		if usr.Uid == "0" {
			configPath = configRoots[1] + configPathSuffix
		} else {
			configPath = configRoots[0] + configPathSuffix
		}
	}
	return nil
}

// ReadConfig finds, processes and loads the configuration file, and sets the global Config item
// to the struct produced. It returns any errors that happen in the process.
func ReadConfig() error {
	err := getConfigPath()
	if err != nil {
		return errors.New(fmt.Sprintf("failed to get config path: %v", err))
	}
	if _, err := os.Stat(configPath); err != nil {
		log.Print("W: Config file doesn't exist. Creating a default one...")
		parentPath, _ := filepath.Split(configPath)
		err := os.MkdirAll(parentPath, 0755)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to get parent of config path %q: %v", configPath, err))
		}
		f, err := os.Create(configPath)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to open config file: %v", err))
		}
		defer f.Close()
		//enc := json.NewEncoder(f)
		//enc.SetIndent("", "\t")
		var logPath, pidPath string
		usr, err := user.Current()
		if err != nil {
			return err
		}
		if usr.Uid == "0" {
			logPath = "/var/log"
			pidPath = "/var/run"
		} else {
			logPath = usr.HomeDir
			pidPath = usr.HomeDir
		}
		Config = config{
			ArchiveRootName:   ArchiveRootName,
			ClientSecretPath:  parentPath + "client_secret.json",
			CreateMissing:     CreateMissing,
			DefaultCategory:   Category,
			ForceRecheck:      ForceRecheck,
			LogFile:           logPath + "/drivesyncd.log",
			PidFile:           pidPath + "/drivesyncd.pid",
			RetryRatio:        RetryRatio,
			RetryStartingRate: RetryStartingRate,
			Verbose:           Verbose,
			UseProxy:          UseProxy,
		}
		b, err := json.MarshalIndent(Config, "", "\t")
		if err != nil {
			return errors.New(fmt.Sprintf("failed to marshal config: %v", err))
		}
		_, err = f.Write(b)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to write the default config file: %v", err))
		}
		log.Printf("I: Default config file created at %q.", configPath)
		return nil
	}
	f, err := os.Open(configPath)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to read config file: %v", err))
	}
	dec := json.NewDecoder(f)
	if err := dec.Decode(&Config); err != nil {
		return errors.New(fmt.Sprintf("failed to decode config file: %v", dec.Decode(&Config)))
	}
	if Config.Target != "" {
		Config.Target = filepath.Clean(Config.Target)
	}
	return nil
}

// ProxySetup sets up the global proxy according to the proxy settings in configuration.
func ProxySetup() {
	if Config.ProxyURL != "" {
		proxyUrl, _ := url.Parse(Config.ProxyURL)
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	} else {
		log.Fatal(`E: Set up to use proxy yet "proxy-url" not set in configuration.`)
	}
}
