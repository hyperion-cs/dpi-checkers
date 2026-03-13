# RU :: DPI-CH (dpi comprehensive checker)
[![dpi-ch release](https://github.com/hyperion-cs/dpi-checkers/actions/workflows/dpich_release.yml/badge.svg)](https://github.com/hyperion-cs/dpi-checkers/actions/workflows/dpich_release.yml)

This is the "big brother" of all other checkers, not limited by the browser sandbox. It is an attempt to create a powerful tool for general-purpose DPI analysis (incl. an improved _tcp 16-20_ checker and much more).<br>
Extremely flexible configuration. Written in golang, builds are [available](https://github.com/hyperion-cs/dpi-checkers/releases/) for windows/macos/linux (android coming soon).
![gif](https://raw.githubusercontent.com/hyperion-cs/dpi-checkers/refs/heads/main/static/images/dpich_v0.2.1.gif)

## Implemented features
- **Who am I?** about your internet connection; aka _whoami checker_;
- **Am I under the CIDR whitelist?** checks if a censor restricts tcp/udp connections by ip subnets; aka _cidrwhitelist_ checker;
- **Comprehensive checks** (_incl. alive and tcp 16-20 restrictions_); aka _webhost checker_:
  - **Popular Web Services** like YouTube, Instagram, Discord, Telegram and others;
  - **Infrastructure Providers** like Cloudflare, Akamai, Hetzner, DigitalOcean and others.
- Modern TUI (aka CLI) with flexible parallel workers;
- Automatic utility update from Github releases;
- Some killer features.

## Killer features
#### ⚡ New method for tcp 16-20
Now, to check for restrictions using the _tcp 16-20_ method, we send data to the host instead of trying to get/download something from it. Research shows that outgoing traffic is restricted by censors in the same way as incoming traffic. This really lowers the requirements for hosts (they just must be able to establish a tcp connection and not close it when they see a stream of data coming from us that's big enough). A similar method is now implemented in the [web version](https://hyperion-cs.github.io/dpi-checkers/ru/tcp-16-20/) of the _tcp 16-20_ checker.

#### ⚡ The era of dynamic: extremely flexible configuration of hosts for checking (for webhost checker)
Now, in the _dpi-ch_ utility, we **do not use** fixed host lists (especially for checking infrastructure providers), incl. for checking _tcp 16-20_, etc. Instead, we obtain such hosts dynamically for each check.
This allows us not to worry about the censor adding our fixed list to their whitelists (to fool our checker), and it also reduces the load on the hosts being checked, since they are unique for each user.

A logical question comes up: how do we set this up? With a new approach — "filters". Each of them returns a set of subnets that satisfy a condition — we will be testing the hosts from them. They are customized in the configuration (see the related section for more details). It may sound complicated at first, but in practice it's a very simple and powerful thing. We can specify not specific hosts (and certainly not endpoints), but much **more general things**. Among them:
- `org(x1,...,xn)`, where `x` is one of the following:
  - _term_ — as a rule, the name of the organization that holds [AS](https://en.wikipedia.org/wiki/Autonomous_system_(Internet)); in fact, this is for a registry-independent search for a substring in the "organization" field within a special registry of all AS;
  - _asn_ — for the specified AS number, an organization name is obtained, and it is then used as _term_ (two-phase search);
  - _ip_ — for the specified IP addr, an organization name is obtained, and it is then used as _term_ (two-phase search).

  Example: `org("hetzner")` — returns a set of subnets that are owned by Hetzner.
- `as(x1,...,xn)`, where `x` is one of the following:
  - _asn_ — AS number;
  - _ip_ — for the specified IP addr, an AS number is obtained, and it is then used as _asn_ (two-phase search);

  Example: `as(24940)` — returns a set of subnets announced by AS24940.
- `country(x1,...,xn)`, where `x` is the [ISO 3166-1 alpha-2](https://en.wikipedia.org/wiki/List_of_ISO_3166_country_codes) country code.

  Example: `country("he", "fi")` — returns a set of all subnets located in Germany or Finland.
- `subnet(x1,...,xn)`, where `x` is one of the following:
  - _subnet_ — just a subnet specified manually in cidr notation (up to `/32` for ipv4);
  - _ip_ — for the specified IP addr, an minimal subnet is obtained (from AS that announce this IP), and it is then used as _subnet_ (two-phase search);

  Example: `subnet("1.1.1.1/32")` — returns a set from one subnet (one IP address).<br>
- `host(x1,...,xn)`, where `x` is a hostname

  Example: `host("google.com")` — returns a set of subnets to which DNS resolves the specified hostname.

Each filter returns a set of subnets that satisfy that filter. They also support multiple arguments and can be combined using logical AND/OR and groups.<br>
Example 1: `(org("hetzner", "digitalocean") && country("de", "fi")) || as(199524, 53667)`<br>
Example 2: `org("hetzner") && country("he")` — returns a set of subnets that are owned by Hetzner and used in hosts in Germany.

The default configuration already includes default filter options for popular web services and infrastructure providers (see below), but we hope you will be able to take full benefit of this flexible feature to suit your needs. By the way, this mechanism inside dpi-ch is called _subnetfilter_ and it works locally without the internet.

## Planned
- [ ] Comprehensive DNS checker (leak test, detection of response hijacking, server hijacking, etc.);
- [ ] Trigger blocks checker;
- [ ] More detailed information in checkers (_statuses, reasons, etc._);
- [ ] TLS certificate hijacking detection in _webhost_ checker;
- [ ] Option to temporarily freeze the list of hosts in _webhost_ checker;
- [ ] Estimation of internet connection speed (including shaping/slowdown detection) in _webhost_ checker;
- [ ] Detecting subnets for CIDR whitelists;
- [ ] Detecting hostnamesfor for SNI whitelists;
- [ ] Integration with [zapret](https://github.com/bol-van/zapret2) to find optimal strategies;
- [ ] Android version (via [Termux](https://en.wikipedia.org/wiki/Termux));
- [ ] Web UI in addition to TUI (backend is already architecturally separated from frontend);
- And a few other minor things.

:bulb: Want anything else? Create an [issue](https://github.com/hyperion-cs/dpi-checkers/issues) or [PR](https://github.com/hyperion-cs/dpi-checkers/pulls).

## Configuration
You can view the default configuration [here](https://github.com/hyperion-cs/dpi-checkers/blob/main/ru/dpi-ch/config/default.yaml) (incl. as an example; some options are internal and are not intended to be changed by users).<br>
In any case, any option in the default configuration can be overwritten by users using a [YAML](https://en.wikipedia.org/wiki/YAML) file. To do this, create a `config.yaml` file near the executable file (the path to the file can be changed with the `--cfg` command line argument). The current configuration structure is available [here](https://github.com/hyperion-cs/dpi-checkers/blob/main/ru/dpi-ch/config/config.go), but below is an attempt to describe it in more detail.

Structure of primary options (internal hidden):
```yaml
debug: # bool; if true, debug info will be saved to the debug.log file near the executable file

checkers: # checkers, available in the dpi-ch utility
  cidrwhitelist: # aka cidrwhitelist checker
    timeout:     # time.Duration; timeout for receiving a response from the next endpoint
    whitelisted: # []string; list of url endpoints that are accessible during cidr restrictions
    regular:     # []string; list of url endpoints that are available during "normal hours"

  webhost: # aka webhost checker
    popular: # []webhost-item; list of popular web services
    infra:   # []webhost-item; list of infrastructure providers

             #  webhost-item structure:
             #	name:            # string; name of hosts group
             #	filter:          # string; filter in subnetfilter notation (see above); if it is one host(), then sni/host will be obtained from there
             #	count:           # int; how many hosts do we need to farm through webhostfarm
             #	port:            # int; port for establishing a tcp connection with hosts
             #	host:            # string; http host header for hosts
             #	sni:             # string; sni for tls handshake
             #	tcp1620-skip:    # bool; skip tcp 16-20 check for hosts
             #	random-hostname: # bool; generate a random http host header for each host

    workers:                # int; number of parallel workers that will find and analyze hosts
    tcp-conn-timeout:       # time.Duration; timeout for establishing a tcp connection
    tls-handshake-timeout:  # time.Duration; timeout for tls handshake
    tcp-read-timeout:       # time.Duration; timeout for reading from a tcp connection (more precisely, from tls over tcp)
    tcp-write-timeout:      # time.Duration; timeout for writing to a tcp connection (more precisely, to tls over tcp)
    tcp-write-buf:          # int; tcp/tls write buffer size (expert warn: only change if you know what you are doing)
    tcp-read-buf:           # int; tcp/tls read buffer size (also only for experts)
    tcp1620-n-bytes:        # int; size of random payload for tcp 16-20 (also only for experts)
    key-log-path:           # string; if set, the (pre)-master-secret log will be written to this path; useful for wireshark
    table-max-visible-rows: # int; number of visible rows in the results table (if there are more, scrolling is available)
    http-static-headers:    # map[string]string; http headers that will be sent as part of requests to hosts

  whoami: # aka whoami checker
    timeout: # time.Duration; total timeout for receiving checker results

# support utilities section:

subnetfilter: # takes filters as input, returns sets of subnets (usually to webhostfarm); works locally without the internet
  workers: # int; number of parallel workers that will process filters

webhostfarm: # takes sets of subnets (usually from subnetfilter), returns suitable hosts (usually for the webhost checker)
  workers:               # int; number of parallel workers that will process sets of subnets
  tcp-conn-timeout:      # time.Duration; timeout for establishing a tcp connection
  tls-handshake-timeout: # time.Duration; timeout for tls handshake

httputil: # used to perform simple http requests
  browser-headers: # map[string]string; http headers that will be sent as part of requests

updater: # used to automatically update the dpi-ch utility and related stuff (e.g., geoip)
  enabled: # bool; if true, updates will be enabled
  period:  # time.Duration; frequency of update checks (by default, no more than once per day)
```

## Similar projects
It so happens that similar projects (unrelated to ours) are under development at the same time, and we are happy to tell you about them.

- [Runnin4ik/dpi-detector](https://github.com/Runnin4ik/dpi-detector) — _DPI detection tool for internet censorship testing_ (Python).

## Third-Party Dependencies
- [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip)
- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- [go4.org/netipx](https://go4.org/netipx)
- [expr-lang/expr](https://github.com/expr-lang/expr)
- [efraction-networking/utls](https://github.com/refraction-networking/utls)
- [spf13/viper](https://github.com/spf13/viper)
