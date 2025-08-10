# RU :: TCP 16-20 DWC (domain whitelist checker)
Allows to find out whitelisted items on DPIs where _TCP 16-20_ blocking method is applied.
This kind of information can be interesting in its own right as well as useful for bypassing limitations.<br>
⚠️ The whitelist means, among other things, that websites (more specifically, domains and subdomains) on this list are loaded without _TCP 16-20_ blocking method limitations (a censor is based on SNI in the TLS handshake and possibly an HTTP `Host` header if _plain HTTP_ is used).

## Ready-to-use results
Not everyone will want to run this script on their own (especially because it can run for quite a long time, and because its implementation is naive and uses a bruteforce method). That's why this work has already been done by the committers of this repository.
The top-10k popular domains based on the list from [OpenDNS](https://github.com/opendns/public-domain-lists/blob/master/opendns-top-domains.txt) (unfortunately, this file was last updated _2014-11-06_, but it's still generally up to date) was used as a input list.

**Last Updated**: _2025-07-02_<br>
**Last File**: [/ru/tcp-16-20_dwc/results/based_on_opendns_2025-07-02.txt](/ru/tcp-16-20_dwc/results/based_on_opendns_2025-07-02.txt)<br>
**Latest stats**: _266_ domains out of _10'000_ (_2.66%_) are whitelisted<br>
**File Format**: _.csv/.md_ table with `| Domain | Provider | Country |` header

⚠️ upd.: We found that whitelists can vary significantly between operators. Nevertheless, on average there are a small number of intersecting results. Thus, you can analyze (using the methodology described here) the necessary operators, find the intersection of the results, and use them as needed.

### Notes
As far as we know, the whitelist is created using the `*.domain.com:*` scheme. Thus, you can (and should?) use subdomains of the found domains (if _site.com_ works, then _foo.site.com_ and _foo.bar.site.com_ will also work).

We also bring to your attention a graph that shows the dependence of being on the whitelist on the place in the top (provided by OpenDNS).
![graph](https://raw.githubusercontent.com/hyperion-cs/dpi-checkers/refs/heads/main/static/images/tcp-16-20_dwc_based_on_opendns_2025-07-02.png)
It can be seen that there is a correlation between these properties (which is generally logical).

## Self-running the script
1. First of all, you have to get an input list with domains to test (the script doesn't get the full whitelist, it just checks your input list to see if each of its elements is included in the DPI whitelist). You can use OpenDNS lists as a starting point (see above), or you can use [Cloudflare Radar](https://radar.cloudflare.com/domains), for example;
2. You will need a remote server in “suspicious” networks (i.e. those limited by _TCP 16-20_ blocking method at your “home” ISP). There, you would need to install a web server with https (a self-signed certificate [_openssl_/etc] is fine, since the script ignores validation) that would respond the same regardless of the SNI passed. It should also send a file of at least 128KB (over the network, including compression) to some path — GET request.<br>
   As such a server you can use _nginx_ with approximately the following configuration:
   ```nginx
   server {
     listen 443 ssl default_server;
     ssl_certificate     /path/to/cert.crt;
     ssl_certificate_key /path/to/cert.key;
     root /var/www/html;
     location / {
       try_files $uri $uri/ =404;
     }
    }
   ```
   A static file can be generated like this (in this case, 1MB in size):
   ```bash
   dd if=/dev/urandom of=/var/www/html/1MB.bin bs=1M count=1
   ```
   \* Don't forget to open https (443) port.
3. Finally, on your local machine (must have Python 3 and the `curl` utility installed) with internet access through an ISP with DPI using the TCP 16-20 blocking method, you can run the script. It is recommended to use a POSIX-compatible OS (Linux, macOS, etc). The script has the following parameters:

   | Parametr | Default | Required | Desc |
   | :-: | :-: | :-: | - |
   | `-i` | _in.txt_ | No |Path to the file with the list of domains to check.|
   | `-o` | _out.txt_ | No |The path to the results file. The domains that are included in the whitelist will be saved.|
   | `-e` | _err.txt_ | No |Error file path.|
   | `-u` | — | Yes |The path for the URL where the static file is located.|
   | `-d` | — | Yes |IP of your destination server from the previous step.|
   | `-t` | `5` | No |Connection/read timeout in seconds.|
   | `-r` | `65535` | No |Upper bound of the range of bytes to be downloaded.|

   Example of a run:
   ```bash
   python domain_whitelist_checker.py -u /1MB.bin -d 1.2.3.4
   ```

The script is single-threaded, but you can parallelize it via e.g. GNU [parallel](https://www.gnu.org/software/parallel/) utility.
Also you can run the result file through [this](/utils/domain2provider.py) script to find out the likely ISPs the domain owners are using, as well as the country.

## Contributing
We would be happy if you could help us improve our checkers through PR or by creating issues.
Also you can star the repository so you don't lose the checkers.
The repository is available [here](https://github.com/hyperion-cs/dpi-checkers).
