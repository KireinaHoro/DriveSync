# DriveSync

DriveSync is a tool for uploading local contents to your Google Drive. Written in Golang with support for `gccgo-6`,
DriveSync aims to support for a wide range of Unix-like distributions. Tested platforms include:

 - macOS High Sierra with go 1.9
 - Gentoo Linux on SPARC with gccgo-6 and gccgo-7

Further tests on other platforms are welcomed.

DriveSync was created to solve the nuisance of uploading BitTorrent downloads to Google Drive automatically.
More using scenarios await your discovery!

## Install

Export `GOPATH` and run the following (same for updating):

```bash
go get -u -v github.com/KireinaHoro/DriveSync/...
```

Don't forget the three dots (`...`) at the end of the above command. It's recommended to add `$GOPATH/bin` to your `$PATH` so
that you can access the executables easily.

## Directory structure that DriveSync maintains

```plain
[Google Drive Root]
├── archive-root/
|   ├── Music/
|   |   ├── My Great Record/
|   |   |   ├── track01.flac
|   |   |   ├── track02.flac
|   |   |   └── ...
|   |   ├── My Great Single.flac
|   |   └── ...
|   ├── Software
|   |   ├── Install panicOS Low Sierra.app.tar.gz
|   |   ├── My Awesome Tools
|   |   |   ├── busybox.tar.gz
|   |   |   ├── bash.tar.gz
|   |   |   └── ...
|   |   └── ...
|   └── ...
├── Your Other Awesome Folders
|   └── ...
└── ...
```

## How it works

DriveSync has two commandline tools available:

 - `drivesync` works in an one-shot manner, while
 - `drivesyncd` forks into the background, scaning a target directory at given frequency

They sync files on your local system to your Google Drive, under `/${ARCHIVE_ROOT}/${DEFAULT_CATEGORY}`. Both commands have
commandline options available. Invoke with `-h` to find out how to use them.

Though DriveSync requires that you provide it with a category (either fixed-default or provided every time on commandline) for now,
support for guessing the most appropriate category according to the object basename is planned. You can learn more about this
[here](https://github.com/KireinaHoro/DriveSync/blob/master/config/category_guessing.go). Pull requests are welcomed.

## Usage & configuration

First of all, obtain your own client secret for DriveSync to run. You can obtain your own `client_secret.json`
[here](https://developers.google.com/drive/v3/web/quickstart/go#step_1_turn_on_the_api_name).

After you've obtained your client secret, launch `drivesync` with `-interactive` to set up the configuration files and credentials.
Note: you need to do this for every user you intend to use the tool with. Edit the configuration file according to your needs.

## Configuration file

Both of the commands read configurations from a JSON file present at:

 - `${XDG_CONFIG_HOME:-"$HOME/.config"}/drivesync/config.json`
 - `/etc/drivesync/config.json`

If the command fails to locate a valid configuration, it will create a sample one with the default values filled in.
The configuration items are explained below.

```go
var DefaultConfig = map[string]interface{}{
	"archive-root":        "archive",                           // the name of the archive root
	"client-secret-path":  "${CONFIG_ROOT}/client_secret.json", // path of client_secret.json
	"create-missing":      false,                               // whether to create missing archive roots or categories
	"default-category":    "Uncategorized",                     // the default category to store content in
	"force-recheck":       true,                                // whether to check if MD5 of local and remote versions of file matches
	"log-file":            "${LOG_ROOT}/drivesyncd.log",        // location of log file
	"pid-file":            "${RUN_ROOT}/drivesyncd.pid",        // location of pid file
	"proxy-url":           "",                                  // http proxy url
	"retry-ratio":         2,                                   // ratio of expotential backoff each time a retry is triggered
	"retry-starting-rate": 1,                                   // starting rate to wait for when retry occurs
	"scan-interval":       "100ms",                             // interval to wait for when scanning for target change
	"target":              "",                                  // path of target directory to be scanned for new objects
	"use-proxy":           false,                               // whether to use proxy for connection
	"verbose":             true,                                // whether to write logs and outputs verbosely
}
```

In the above default config,

 - `CONFIG_ROOT` will be expanded with `/etc/drivesync` if the user invoking the command to create the config file is root,
   or `${XDG_CONFIG_HOME:-"$HOME/.config"}/drivesync` otherwise;
 - `LOG_ROOT` will be `/var/log` and `RUN_ROOT` will be `/var/run` if invoked as root, or both will be `${HOME}/drivesync`
   otherwise.

__NOTE:__ for ease of use, DriveSync will fall back to the permissive path listed above if the one configured in `config.json`
is not available for writing for the caller. This behavior is for scenarios of users trying to launch `drivesync` or their own
instance of `drivesyncd` without a user-specific `config.json`, which, if without this behavior, would fail due to missing permissions
to write to system paths.

You can reload the configuration file for a running `drivesyncd` with:

```bash
drivesyncd -s reload
```

...or send SIGHUP to it. SIGTERM and SIGQUIT can also be sent via the `-s` switch. Use `drivesyncd -h` to find out more.

**NOTE:** due to limitations of the watcher API, `target` and `scan-interval` options won't get reloaded with a configuration
file reload. You'll need to restart the daemon to reload these options.

## License

DriveSync is licensed under AGPLv3. The full license text is available in the repository root, named LICENSE-AGPLv3.txt .

## Donations

If you find this work helpful, consider buying me a glass of beer :) Accepted payment methods listed below:

 - PayPal: i@jsteward.moe
 - BTC: 13jTGFvjh7DAwiHZzxpaiqfehVnX2CWncC
