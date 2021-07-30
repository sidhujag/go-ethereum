// Copyright 2017 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/log"
)

// explorerDockerfile is the Dockerfile required to run a block explorer.
var explorerDockerfile = `
FROM sidhujag/syscoin-core:latest as syscoin-alpine
ARG COIN={{.Coin}}
ARG BLOCK_TRANSFORMER={{.BlockTransformer}}
ARG CSS_PRIMARY={{.CssPrimary}}
ARG CSS_SECONDARY={{.CssSecondary}}
ARG CSS_TERTIARY={{.CssTertiary}}
ARG CSS_PRIMARY_DARK={{.CssPrimaryDark}}
ARG CSS_SECONDARY_DARK={{.CssSecondaryDark}}
ARG CSS_TERTIARY_DARK={{.CssTertiaryDark}}
ARG CSS_FOOTER_BACKGROUND={{.CssFooterBackground}}
ARG CSS_FOOTER_TEXT={{.CssFooterText}}
FROM sidhujag/blockscout:latest

ENV SYSCOIN_DATA=/home/syscoin/.syscoin
ENV SYSCOIN_VERSION=4.3.99
ENV SYSCOIN_PREFIX=/opt/syscoin-${SYSCOIN_VERSION}

RUN rm /usr/local/bin/geth
COPY --from=syscoin-alpine ${SYSCOIN_DATA}/* /opt/app/.syscoin/
COPY --from=syscoin-alpine ${SYSCOIN_PREFIX}/bin/* /usr/local/bin/
ENV NETWORK={{.Network}} \
    SUBNETWORK={{.SubNetwork}} \
    COINGECKO_COIN_ID={{.CoingeckoID}} \
    COIN={{.Coin}} \
    LOGO={{.Logo}} \
    LOGO_FOOTER={{.LogoFooter}} \
    LOGO_TEXT={{.LogoText}} \
    CHAIN_ID={{.NetworkID}} \
    HEALTHY_BLOCKS_PERIOD={{.HealthyBlockPeriod}} \
    SUPPORTED_CHAINS={{.SupportedChains}} \
    BLOCK_TRANSFORMER={{.BlockTransformer}} \
    SHOW_TXS_CHART={{.ShowTxChart}} \
    DISABLE_EXCHANGE_RATES={{.DisableExchangeRates}} \
    SHOW_PRICE_CHART={{.ShowPriceChart}} \
    ETHEREUM_JSONRPC_HTTP_URL={{.HttpUrl}} \
    ETHEREUM_JSONRPC_WS_URL={{.WsUrl}}

RUN \
    echo '/usr/local/bin/docker-entrypoint.sh postgres &' >> explorer.sh && \
    echo 'sleep 5' >> explorer.sh && \
    echo 'mix do ecto.drop --force, ecto.create, ecto.migrate' >> explorer.sh && \
    echo 'mix phx.server &' >> explorer.sh && \
    echo $'LC_ALL=C syscoind {{if eq .NetworkID 58}}--testnet{{end}} --addnode=3.15.199.152 --datadir=/opt/app/.syscoin --disablewallet --zmqpubnevm="tcp://127.0.0.1:1111" --gethcommandline=--syncmode="full" --gethcommandline=--gcmode="archive" --gethcommandline=--port={{.EthPort}} --gethcommandline=--bootnodes={{.Bootnodes}} --gethcommandline=--ethstats={{.Ethstats}} --gethcommandline=--cache=512 --gethcommandline=--http --gethcommandline=--http.api="net,web3,eth,shh,debug,network,txpool" --gethcommandline=--http.corsdomain="*" --gethcommandline=--http.vhosts="*" --gethcommandline=--ws --gethcommandline=--ws.origins="*" --gethcommandline=--exitwhensynced' >> explorer.sh && \
    echo $'LC_ALL=C exec syscoind {{if eq .NetworkID 58}}--testnet{{end}} --addnode=3.15.199.152 --datadir=/opt/app/.syscoin --disablewallet --zmqpubnevm="tcp://127.0.0.1:1111" --gethcommandline=--syncmode="full" --gethcommandline=--gcmode="archive" --gethcommandline=--port={{.EthPort}} --gethcommandline=--bootnodes={{.Bootnodes}} --gethcommandline=--ethstats={{.Ethstats}} --gethcommandline=--cache=512 --gethcommandline=--http --gethcommandline=--http.api="net,web3,eth,shh,debug,network,txpool" --gethcommandline=--http.corsdomain="*" --gethcommandline=--http.vhosts="*" --gethcommandline=--ws --gethcommandline=--ws.origins="*" &' >> explorer.sh

ENTRYPOINT ["/bin/sh", "explorer.sh"]
`

// explorerComposefile is the docker-compose.yml file required to deploy and
// maintain a block explorer.
var explorerComposefile = `
version: '2'
services:
    explorer:
        build: .
        image: {{.Network}}/explorer
        container_name: {{.Network}}_explorer_1
        ports:
            - "{{.EthPort}}:{{.EthPort}}"
            - "{{.SysPort1}}:{{.SysPort1}}"
            - "{{.SysPort2}}:{{.SysPort2}}"
            - "{{.SysPort3}}:{{.SysPort3}}"
            - "{{.EthPort}}:{{.EthPort}}/udp"{{if not .VHost}}
            - "{{.WebPort}}:4000"{{end}}
        environment:
            - ETH_PORT={{.EthPort}}
            - ETH_NAME={{.EthName}}
            - BLOCK_TRANSFORMER={{.Transformer}}{{if .VHost}}
            - VIRTUAL_HOST={{.VHost}}
            - VIRTUAL_PORT=4000{{end}}
        volumes:
            - {{.Datadir}}:/opt/app/.ethereum
            - {{.DBDir}}:/var/lib/postgresql/data
        logging:
          driver: "json-file"
          options:
            max-size: "1m"
            max-file: "10"
        restart: always
`
// logoSVG is the SVG logo for our explorer on the header and footer
// nolint: misspell
var logoSVG = []byte(`"<?xml version=\"1.0\" encoding=\"UTF-16\"?>
<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd\">
<!-- Creator: CorelDRAW 2020 (64-Bit) -->
<svg xmlns=\"http://www.w3.org/2000/svg\" xml:space=\"preserve\" width=\"1145px\" height=\"263px\" version=\"1.1\" style=\"shape-rendering:geometricPrecision; text-rendering:geometricPrecision; image-rendering:optimizeQuality; fill-rule:evenodd; clip-rule:evenodd\"
viewBox=\"0 0 1151.04 264.17\"
 xmlns:xlink=\"http://www.w3.org/1999/xlink\"
 xmlns:xodm=\"http://www.corel.com/coreldraw/odm/2003\">
 <defs>
  <style type=\"text/css\">
   <![CDATA[
    .fil5 {fill:none}
    .fil2 {fill:#22A9DA}
    .fil4 {fill:#257DB8}
    .fil3 {fill:#008DD1}
    .fil1 {fill:#40494E;fill-rule:nonzero}
    .fil0 {fill:#2D8DCB;fill-rule:nonzero}
   ]]>
  </style>
 </defs>
 <g id=\"Layer_x0020_1\">
  <metadata id=\"CorelCorpID_0Corel-Layer\"/>
  <path class=\"fil0\" d=\"M375.85 188.37c0,12.05 -8.95,18.42 -21.77,18.42l-12.31 0 0 15.49 -10.41 0 0 -51.63 23.41 0c12.99,0 21.08,6.45 21.08,17.72zm-21.51 -8.6l0 0 -12.57 0 0 18.16 12.65 0c6.71,0 10.76,-2.85 10.76,-9.13 0,-5.93 -4.13,-9.03 -10.84,-9.03zm38.2 42.51l0 0 0 -51.63 10.41 0 0 42.51 27.53 0 0 9.12 -37.94 0zm73.74 -51.63l0 0 1.8 0 26.59 51.63 -11.79 0 -3.35 -7.06 -24.7 0 -3.27 7.06 -11.79 0 26.51 -51.63zm1.2 18.84l0 0 -0.69 0 -7.83 16.87 -0.6 1.12 17.55 0 -0.6 -1.12 -7.83 -16.87zm30.54 -18.84l0 0 44.06 0 0 9.12 -16.78 0 0 42.51 -10.41 0 0 -42.51 -16.87 0 0 -9.12zm59.98 0l0 0 40.1 0 0 9.12 -29.69 0 0 12.56 25.47 0 0 8.52 -25.47 0 0 21.43 -10.41 0 0 -51.63zm53.25 25.81l0 0c0,-14.88 10.42,-26.93 27.28,-26.93 16.96,0 27.37,12.05 27.37,26.93 0,14.89 -10.41,26.94 -27.37,26.94 -16.86,0 -27.28,-12.05 -27.28,-26.94zm43.98 0l0 0c0,-9.81 -6.97,-16.95 -16.7,-16.95 -9.63,0 -16.52,7.14 -16.52,16.95 0,9.81 6.89,16.96 16.52,16.96 9.73,0 16.7,-7.15 16.7,-16.96zm28.47 -25.81l0 0 23.32 0c13.08,0 21.17,6.45 21.17,17.72 0,7.66 -3.78,12.91 -9.72,15.75l0 0.78 12.3 16.34 0 1.04 -11.79 0 -11.44 -15.75 -13.43 0 0 15.75 -10.41 0 0 -51.63zm22.98 9.12l0 0 -12.57 0 0 17.98 12.83 0c6.54,0 10.49,-2.92 10.49,-8.95 0,-5.93 -4.04,-9.03 -10.75,-9.03zm83.37 12.05l0 0 -14.97 15.14 -2.06 0 -15.06 -15.14 0 30.46 -10.42 0 0 -51.63 2.84 0 23.5 23.14 23.75 -23.14 2.75 0 0 51.63 -10.33 0 0 -30.46z\"/>
  <path class=\"fil1\" d=\"M341.74 115.61c0,0 12.42,9.12 27.84,9.12 11.48,0 16.04,-4.56 16.04,-10.54 0,-6.13 -4.09,-9.27 -20.44,-13.84 -21.4,-5.97 -32.56,-13.52 -32.56,-28.62 0,-16.83 12.58,-27.99 36.8,-27.99 25.32,0 35.7,9.75 35.7,9.75l-10.22 15.57c0,0 -10.07,-7.86 -25.48,-7.86 -10.38,0 -15.41,3.61 -15.41,9.43 0,5.66 4.87,8.18 19.66,12.27 23.11,6.13 33.65,14.31 33.65,29.56 0,16.52 -11.48,29.73 -37.27,29.73 -25.95,0 -38.69,-12.11 -38.69,-12.11l10.38 -14.47zm167.8 -69.83l0 0 22.02 0 -35.7 58.35 0 36.01 -19.03 0 0 -36.17 -35.39 -58.19 22.02 0 19.5 32.87 2.68 5.98 1.73 0 2.83 -5.98 19.34 -32.87zm65.26 69.83l0 0c0,0 12.42,9.12 27.84,9.12 11.48,0 16.04,-4.56 16.04,-10.54 0,-6.13 -4.09,-9.27 -20.44,-13.84 -21.4,-5.97 -32.56,-13.52 -32.56,-28.62 0,-16.83 12.58,-27.99 36.8,-27.99 25.32,0 35.7,9.75 35.7,9.75l-10.22 15.57c0,0 -10.07,-7.86 -25.48,-7.86 -10.38,0 -15.41,3.61 -15.41,9.43 0,5.66 4.87,8.18 19.66,12.27 23.11,6.13 33.65,14.31 33.65,29.56 0,16.52 -11.48,29.73 -37.27,29.73 -25.95,0 -38.69,-12.11 -38.69,-12.11l10.38 -14.47zm190.91 14.94l0 0c0,0 -10.53,11.64 -33.33,11.64 -30.83,0 -50.17,-22.02 -50.17,-49.23 0,-27.2 19.34,-49.22 50.17,-49.22 22.96,0 33.33,11.64 33.33,11.64l-10.85 14.62c0,0 -9.75,-8.02 -22.17,-8.02 -18.09,0 -30.83,13.05 -30.83,30.98 0,17.93 12.74,31.14 30.83,31.14 14.31,0 22.17,-8.18 22.17,-8.18l10.85 14.63zm37.27 -37.59l0 0c0,-27.2 19.03,-49.22 49.86,-49.22 30.98,0 50.01,22.02 50.01,49.22 0,27.21 -19.03,49.23 -50.01,49.23 -30.83,0 -49.86,-22.02 -49.86,-49.23zm80.37 0l0 0c0,-17.93 -12.74,-30.98 -30.51,-30.98 -17.62,0 -30.2,13.05 -30.2,30.98 0,17.93 12.58,30.98 30.2,30.98 17.77,0 30.51,-13.05 30.51,-30.98zm86.8 47.18l0 0 -19.03 0 0 -94.36 19.03 0 0 94.36zm141.07 0l0 0 -4.56 0 -63.23 -57.56 0 57.56 -19.03 0 0 -94.36 4.72 0 63.22 56.3 0 -56.14 18.88 0 0 94.2z\"/>
  <g id=\"_1983292008768\">
   <path class=\"fil2\" d=\"M234.58 206.96c-6.14,5.34 -13.12,10.2 -20.79,14.44 1.43,-1.13 -3.04,-5.84 -1.69,-7.04 33.84,-29.43 41.58,-67.32 27.92,-100.65 -1.85,-4.49 2.04,-6.3 2.06,-10.42 31.17,29.45 27.81,72.98 -7.5,103.67z\"/>
   <path class=\"fil3\" d=\"M242.08 103.29c-11.44,-10.82 -27.49,-19.74 -48.32,-25.44 -0.09,-0.02 -0.17,-0.04 -0.27,-0.06 0.1,0.02 0.19,0.04 0.29,0.07 6.84,1.79 7.37,9.71 12.08,13.7 10.08,8.56 12.84,19.51 13.87,33.98 1.55,21.53 1.43,42.83 -14.3,61.79 -8.25,9.96 -18.67,19.01 -30.99,24.32 -0.91,0.41 -1.81,0.81 -2.75,1.2 -0.15,0.03 -0.26,0.11 -0.4,0.14 -5.92,2.38 -12.5,4.32 -19.77,5.67 -25.51,4.82 -59.24,2.81 -100.06,-10.16 16.3,12.57 33.76,20.94 51.38,25.76 19.29,5.3 38.77,6.4 57.19,4.2 0.04,0 0.11,0 0.15,-0.03 2.74,-0.29 5.44,-0.7 8.11,-1.17 10.71,-1.86 20.98,-4.86 30.52,-8.73l0.03 0c5.19,-2.12 10.2,-4.5 14.95,-7.13 1.43,-1.13 2.85,-2.3 4.2,-3.51 38.41,-33.4 47.11,-78.72 24.09,-114.6z\"/>
   <path class=\"fil4\" d=\"M171.69 212.84c47.72,-19.44 50.28,-68.71 -5.17,-96.45 -40.28,-20.15 -22.49,-52.14 27.26,-38.53 55.57,14.59 39.13,111.37 -22.09,134.98z\"/>
   <path class=\"fil2\" d=\"M245.75 51.22c-52.44,-16.67 -93.13,-15.24 -119.82,-4.49 -0.13,0.05 -0.26,0.09 -0.4,0.15 -39.68,16.18 -48.14,52.97 -17.97,80.86 -7.95,-6.6 -17.02,-13.61 -20.95,-22.52 -5.01,-11.38 -6.52,-25.47 -4.51,-37.31 2.71,-16.02 12.84,-30.46 29.5,-39.29 6.63,-3.51 16.61,-6.02 25.59,-7.38 35.27,-4.21 74.45,3.62 108.56,29.98z\"/>
   <path class=\"fil3\" d=\"M116.44 184.53c-3.44,-1.12 -7.32,-1.65 -10.51,-2.93 -40.5,-16.29 -62.14,-46.59 -61.36,-76.49 0.55,-21.25 13.01,-42.76 33.4,-60.5 5.78,-5.01 13.38,-9.25 20.39,-13.41 0.02,-0.02 0.03,-0.02 0.05,-0.02 9.54,-3.88 19.8,-6.86 30.52,-8.73 2.72,-0.47 5.48,-0.87 8.27,-1.2 -3.3,0.5 -6.42,1.14 -9.38,1.98 -52.42,14.25 -55.56,75.25 -20.25,104.51 0,0 0.01,0 0.03,0.02 6.1,5.63 13.78,10.92 23.1,15.57 36.53,18.29 25.29,46.31 -14.26,41.2z\"/>
   <path class=\"fil4\" d=\"M116.44 184.53c-4.04,-0.52 -8.39,-1.39 -13,-2.66 -77.66,-21.22 -89.05,-87.17 -40.82,-129.12 9.96,-8.65 22.11,-16.02 35.74,-21.55 -7.01,4.16 -13.47,8.79 -19.25,13.8 -51.35,44.67 -41.12,114.12 37.33,139.53z\"/>
  </g>
  <rect class=\"fil5\" width=\"1151.04\" height=\"264.17\"/>
 </g>
</svg>"`)
// deployExplorer deploys a new block explorer container to a remote machine via
// SSH, docker and docker-compose. If an instance with the specified network name
// already exists there, it will be overwritten!
func deployExplorer(client *sshClient, network string, bootnodes []string, config *explorerInfos, nocache bool, isClique bool) ([]byte, error) {
	// Generate the content to upload to the server
	workdir := fmt.Sprintf("%d", rand.Int63())
	files := make(map[string][]byte)
	transformer := "base"
	if isClique {
		transformer = "clique"
	}
	dockerfile := new(bytes.Buffer)
	subNetwork := ""
	showPriceChart := "true"
	disableExchangeRates := "false"
	supportedChains := `[{"title":"Syscoin","url":"https://blockscout.com/rsk/mainnet"}]`
	if config.node.network == 58 {
		subNetwork = "Tanenbaum"
		disableExchangeRates = "false"
		showPriceChart = "true"
		supportedChains = `[{"title":"Tanenbaum","url":"https://blockscout.com/rsk/mainnet","test_net?":true}]`
	}
	host := config.host
	if host == "" {
		host = client.server
	}
	template.Must(template.New("").Parse(explorerDockerfile)).Execute(dockerfile, map[string]interface{}{
		"NetworkID": config.node.network,
		"Bootnodes": strings.Join(bootnodes, ","),
		"Ethstats":  config.node.ethstats,
		"EthPort":   config.node.port,
		"HttpUrl":   "http://localhost:8545",
		"WsUrl":   "ws://localhost:8546",
		"Network":   "Syscoin",
		"SubNetwork": subNetwork,
		"CoingeckoID":   "syscoin",
		"Coin":   "SYS",
		"Logo":   "/images/blockscout_logo_sys.svg",
		"LogoFooter":   "/images/blockscout_logo_sys.svg",
		"LogoText":   "NEVM",
		"HealthyBlockPeriod": 34500000,
		"SupportedChains": supportedChains,
		"BlockTransformer": transformer,
		"ShowTxChart": "true",
		"DisableExchangeRates": disableExchangeRates,
		"ShowPriceChart": showPriceChart,
		"CssPrimary": "#257db8",
		"CssSecondary": "#87e1a9",
		"CssTertiary": "#6fB8df",
		"CssPrimaryDark": "#6fB8df",
		"CssSecondaryDark": "#87e1a9",
		"CssTertiaryDark": "#257db8",
		"CssFooterBackground": "#101d49",
		"CssFooterText": "#6fB8df",
	})
	files[filepath.Join(workdir, "Dockerfile")] = dockerfile.Bytes()

	composefile := new(bytes.Buffer)
	template.Must(template.New("").Parse(explorerComposefile)).Execute(composefile, map[string]interface{}{
		"Network":     network,
		"VHost":       config.host,
		"Ethstats":    config.node.ethstats,
		"Datadir":     config.node.datadir,
		"DBDir":       config.dbdir,
		"EthPort":     config.node.port,
		"SysPort1":    8369,
		"SysPort2":    18369,
		"SysPort3":    18444,
		"EthName":     config.node.ethstats[:strings.Index(config.node.ethstats, ":")],
		"WebPort":     config.port,
		"Transformer": transformer,
	})
	files[filepath.Join(workdir, "docker-compose.yaml")] = composefile.Bytes()
	files[filepath.Join(workdir, "genesis.json")] = config.node.genesis
	files["/images/blockscout_logo_sys.svg"] = logoSVG
	// Upload the deployment files to the remote server (and clean up afterwards)
	if out, err := client.Upload(files); err != nil {
		return out, err
	}
	defer client.Run("rm -rf " + workdir)

	// Build and deploy the boot or seal node service
	if nocache {
		return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s build --pull --no-cache && docker-compose -p %s up -d --force-recreate --timeout 60", workdir, network, network))
	}
	return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s up -d --build --force-recreate --timeout 60", workdir, network))
}

// explorerInfos is returned from a block explorer status check to allow reporting
// various configuration parameters.
type explorerInfos struct {
	node  *nodeInfos
	dbdir string
	host  string
	port  int
}

// Report converts the typed struct into a plain string->string map, containing
// most - but not all - fields for reporting to the user.
func (info *explorerInfos) Report() map[string]string {
	report := map[string]string{
		"Website address ":        info.host,
		"Website listener port ":  strconv.Itoa(info.port),
		"Ethereum listener port ": strconv.Itoa(info.node.port),
		"Ethstats username":       info.node.ethstats,
	}
	return report
}

// checkExplorer does a health-check against a block explorer server to verify
// whether it's running, and if yes, whether it's responsive.
func checkExplorer(client *sshClient, network string) (*explorerInfos, error) {
	// Inspect a possible explorer container on the host
	infos, err := inspectContainer(client, fmt.Sprintf("%s_explorer_1", network))
	if err != nil {
		return nil, err
	}
	if !infos.running {
		return nil, ErrServiceOffline
	}
	// Resolve the port from the host, or the reverse proxy
	port := infos.portmap["4000/tcp"]
	if port == 0 {
		if proxy, _ := checkNginx(client, network); proxy != nil {
			port = proxy.port
		}
	}
	if port == 0 {
		return nil, ErrNotExposed
	}
	// Resolve the host from the reverse-proxy and the config values
	host := infos.envvars["VIRTUAL_HOST"]
	if host == "" {
		host = client.server
	}
	// Run a sanity check to see if the devp2p is reachable
	p2pPort := infos.portmap[infos.envvars["ETH_PORT"]+"/tcp"]
	if err = checkPort(host, p2pPort); err != nil {
		log.Warn("Explorer node seems unreachable", "server", host, "port", p2pPort, "err", err)
	}
	if err = checkPort(host, port); err != nil {
		log.Warn("Explorer service seems unreachable", "server", host, "port", port, "err", err)
	}
	// Assemble and return the useful infos
	stats := &explorerInfos{
		node: &nodeInfos{
			datadir:  infos.volumes["/opt/app/.ethereum"],
			port:     infos.portmap[infos.envvars["ETH_PORT"]+"/tcp"],
			ethstats: infos.envvars["ETH_NAME"],
		},
		dbdir: infos.volumes["/var/lib/postgresql/data"],
		host:  host,
		port:  port,
	}
	return stats, nil
}
