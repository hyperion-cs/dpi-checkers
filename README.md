# DPI Checkers
🚀 This repository contains checkers that allow you to determine if your “home” ISP has DPI, as well as the specific methods (and their parameters) the censor uses for limitations.

> [!WARNING]  
> All content in this repository is provided **for research and educational purposes only**.  
> You are **solely responsible** for ensuring that your use of any code, data, or information from this repository complies with all applicable laws and regulations in your jurisdiction.  
> The authors and contributors **assume no liability** for any misuse or violations arising from the use of this materials.

## Checkers list
- **RU :: TCP 16-20** => [https://hyperion-cs.github.io/dpi-checkers/ru/tcp-16-20](https://hyperion-cs.github.io/dpi-checkers/ru/tcp-16-20)<br>
  Allows to detect _TCP 16-20_ blocking method in Russia. The tests use publicly available APIs of popular services hosted by providers whose subnets are potentially subject to limitations. The testing process runs right in your browser and the source code is available. VPN should be disabled during the check.<br>
  This checker has optional GET parameters:
  | name | type |	default	| description |
  |:-:|:-:|:-:|-|
  | timeout | int | `5000` | Timeout for connecting/fetching data from endpoint (in ms). |
  | url | string | — | A custom endpoint to check in addition to the default ones (e.g. your steal-oneself server). The testing endpoint should allow cross-origin requests and provide at least 64KB of data (over the network, including compression, etc.). When not specified, the `times` and `provider` options are ignored. |
  | times | int | `1` | How many times to access the endpoint in a single HTTP connection (_keep-alived_). |
  | provider | string | _Custom_ | Provider name (you can set any name). |

  See [here](https://github.com/net4people/bbs/issues/490) for details on this blocking method.
- **RU :: TCP 16-20 DWC** (domain whitelist checker)<br>
  Allows to find out whitelisted items on DPIs where _TCP 16-20_ blocking method is applied. This kind of information can be interesting in its own right as well as useful for bypassing limitations.<br>
  A list of domains is required as input. Also requires _Python 3_, the _curl_ utility, and a specially configured server on "limited" networks. See [here](ru/tcp-16-20_dwc) for details (ready-to-use results are also available for download there).

## Contributing
We would be happy if you could help us improve our checkers through PR or by creating issues.
Also you can star the repository so you don't lose the checkers.
The repository is available [here](https://github.com/hyperion-cs/dpi-checkers).
