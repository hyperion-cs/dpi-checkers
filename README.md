# DPI Checkers
🚀 This repository contains checkers that allow you to determine if your "home" ISP has DPI, as well as the specific methods (and their parameters) the censor uses for limitations.

> [!WARNING]  
> All content in this repository is provided **for research and educational purposes only**.  
> You are **solely responsible** for ensuring that your use of any code, data, or information from this repository complies with all applicable laws and regulations in your jurisdiction.  
> The authors and contributors **assume no liability** for any misuse or violations arising from the use of this materials.

## Checkers list
- **RU :: IPv4 Whitelisted Subnets** => [https://hyperion-cs.github.io/dpi-checkers/ru/ipv4-whitelisted-subnets](https://hyperion-cs.github.io/dpi-checkers/ru/ipv4-whitelisted-subnets)<br>
  Allows to detect [IPv4 subnets](https://en.wikipedia.org/wiki/Subnet) from the so-called "whitelist" in cases where a censor restricts TCP/UDP/etc connections by IP subnets (aka [CIDR](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing) censorship). There are three control buttons:<br>
  - _Cache_ — fetch and cache suitable IPv4 subnets in the client browser (_local storage_) for further tests. They are saved even after reloading the checker's web page, exiting a browser, etc. This process uses services that are almost certainly not on the whitelist, so it is wise to run it when your provider does not use whitelists (e.g., your "home" ISP's Wi-Fi). This process can only be repeated when you want to update the list of testable subnets of suitable [ASes](https://en.wikipedia.org/wiki/Autonomous_system_(Internet)) (and they change quite rarely);
  - _Check_ — check suitable subnets if they are on the whitelist;
  - _Save_ — save the check results to a _.csv_ file.

  This checker has optional _GET_ parameters:
  | name | type |	default	| description |
  |:-:|:-:|:-:|-|
  | timeout | int | `5000` | Timeout for connecting/fetching data from host (in ms). |
  | sn_sample_size | int | `25` | The number of random unique hosts that will be checked for each suitable subnet. |
  | sn_alive_min | int | `3` | The minimum number of "alive" hosts in a subnet to declare it as whitelisted. |
  | sn_only_24_prefix | bool | `true` | Check only subnets with the `/24` prefix in each AS (this is usually preferable, as a censor is unlikely to allow larger subnets). |

  :warning: There are some nuances to be noted:
  - Not all subnets on the _Internet_ are tested, only those _AS_ subnets that could potentially be on the whitelist and that could potentially be available to the "customer";
  - There may be _false negative_ results, as selective checks are used for performance reasons + a test HTTP(S) HEAD request is sent to port `443` for selected hosts in each subnet;
  - This checker will not work if a censor, in addition to subnet restrictions, also restricts [TLS SNI](https://en.wikipedia.org/wiki/Server_Name_Indication) (_unfortunately, the browser sandbox is unable to spoof this parameter_);
  - If you are using mobile internet, don't worry about large traffic usage (_it will use a couple of megabytes at maximum_);
  - It is prohibited to minimize the browser or lock the screen on phones during the check (_however, you can share Wi-Fi from your phone to your computer — this is more convenient_);
  - Even with performance optimizations, the checker can take quite a while to run (_several tens of minutes_). In the worst case, the time ≈ "_number of suitable subnets_" × `timeout` (_see above_). 
  

- **RU :: TCP 16-20** => [https://hyperion-cs.github.io/dpi-checkers/ru/tcp-16-20](https://hyperion-cs.github.io/dpi-checkers/ru/tcp-16-20)<br>
  Allows to detect _TCP 16-20_ blocking method in Russia. The tests use publicly available APIs of popular services hosted by providers whose subnets are potentially subject to limitations. The testing process runs right in your browser and the source code is available. VPN should be disabled during the check.<br>
  This checker has optional _GET_ parameters:
  | name | type |	default	| description |
  |:-:|:-:|:-:|-|
  | timeout | int | `5000` | Timeout for connecting/fetching data from endpoint (in ms). |
  | url | string | — | A custom endpoint to check in addition to the default ones (e.g. your steal-oneself server). The testing endpoint should allow [cross-origin requests](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS) and provide at least 24KB of data (over the network, including compression, etc.). When not specified, the `thrBytes`, `times` and `provider` options are ignored. |
  | thrBytes | int | `65536` | The minimum number of bytes in an uncompressed response from a server for marking an endpoint as "not detected". It is estimated using the [http compression prober](https://github.com/hyperion-cs/dpi-checkers/blob/main/utils/http_compression_prober.py) utility in the current repo (you should take the max `decompr` value from there). Please note that the default value is only suitable for endpoints with binary data (without network compression). |
  | times | int | `1` | How many times to access the endpoint in a single HTTP connection (_keep-alived_). |
  | provider | string | _Custom_ | Provider name (you can set any name). |

  See [here](https://github.com/net4people/bbs/issues/490) for details on this blocking method.
- **RU :: TCP 16-20 DWC** (domain whitelist checker)<br>
  Allows to find out whitelisted items on DPIs where _TCP 16-20_ blocking method is applied. This kind of information can be interesting in its own right as well as useful for bypassing limitations.<br>
  A list of domains is required as input. Also requires _Python 3_, the _curl_ utility, and a specially configured server on "limited" networks. See [here](ru/tcp-16-20_dwc) for details (ready-to-use results are also available for download there).

## Contributing
We would be happy if you could help us improve our checkers through PR or by creating issues (please use only English for international communication).
Also you can star the repository so you don't lose the checkers.
The repository is available [here](https://github.com/hyperion-cs/dpi-checkers).
