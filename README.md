# pullout

Pull every known URL, path and endpoint out of a single host.

`pullout` runs the URL-discovery tools you already have installed, in parallel,
then scopes, normalises and de-duplicates their combined output into one clean
stream. It does not enumerate subdomains.

## Install

```
go install github.com/armamini/pullout@latest
```

`pullout` has no Go dependencies. It drives these tools when they are on your
`PATH` and skips any that are missing:

| Tool | Kind | Install |
|------|------|---------|
| [gau](https://github.com/lc/gau) | passive | `go install github.com/lc/gau/v2/cmd/gau@latest` |
| [waybackurls](https://github.com/tomnomnom/waybackurls) | passive | `go install github.com/tomnomnom/waybackurls@latest` |
| [gowaybackgo](https://github.com/OoS-MaMaD/gowaybackgo) | passive | `go install github.com/OoS-MaMaD/gowaybackgo@latest` |
| [waymore](https://github.com/xnl-h4ck3r/waymore) | passive | `pipx install waymore` |
| [urlfinder](https://github.com/projectdiscovery/urlfinder) | passive | `go install github.com/projectdiscovery/urlfinder/cmd/urlfinder@latest` |
| [katana](https://github.com/projectdiscovery/katana) | **active** | `go install github.com/projectdiscovery/katana/cmd/katana@latest` |

Check what is available with `pullout -list`.

## Usage

```
pullout [flags] <domain> [domain...]
cat hosts.txt | pullout [flags]
```

```
pullout subdomain-1.example.com
pullout -passive subdomain-1.example.com > urls.txt
pullout -paths subdomain-1.example.com | sort > wordlist.txt
pullout -tools gau,katana -timeout 3m example.com
```

| Flag | Default | Description |
|------|---------|-------------|
| `-subs` | off | Also keep URLs on subdomains of the target. |
| `-paths` | off | Print unique paths instead of full URLs. |
| `-passive` | off | Archive sources only; never send traffic to the target. |
| `-tools a,b` | all installed | Only run these tools. |
| `-exclude a,b` | none | Skip these tools. |
| `-timeout` | `10m` | Maximum run time per tool, per target. Partial results are kept. |
| `-v` | off | Show tool stderr and per-tool statistics. |
| `-list` | | List supported tools and whether they are installed. |

Results go to stdout as they are found; diagnostics go to stderr. The exit
status is non-zero if every tool failed for a target.

## Behaviour

- **Scope.** Only `http(s)` URLs whose host is exactly the target are printed
  (or the target and its subdomains with `-subs`), whatever the underlying
  tools return.
- **Normalisation.** Scheme and host are lowercased; default ports, userinfo
  and fragments are dropped, so the same endpoint from different tools appears
  once.
- **Active vs passive.** `katana` crawls the target directly. Use `-passive`
  when you must not touch it, and only run active crawling against hosts you
  are authorised to test.
- **Rate limits.** Most passive tools query the Wayback Machine. For very
  large hosts expect some `429`s (visible with `-v`); the other sources still
  complete.
- **API keys.** Tools read their own config files (`~/.gau.toml`, waymore's
  `config.yml`, urlfinder's `provider-config.yaml`). Adding URLScan /
  VirusTotal keys there increases coverage.
