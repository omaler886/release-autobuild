package v2rayxhttp

import E "github.com/sagernet/sing/common/exceptions"

const (
	ModeAuto        = "auto"
	ModePacketUp    = "packet-up"
	ModeStreamUp    = "stream-up"
	ModeStreamOne   = "stream-one"
	PlacementPath   = "path"
	PlacementQuery  = "query"
	PlacementHeader = "header"
	PlacementCookie = "cookie"
	PlacementBody   = "body"
)

func normalizeMode(mode string) (string, error) {
	switch mode {
	case "":
		return ModeAuto, nil
	case ModeAuto, ModePacketUp, ModeStreamUp, ModeStreamOne:
		return mode, nil
	default:
		return "", E.New("unsupported xhttp mode: ", mode)
	}
}

func normalizePlacement(name string, value string) (string, error) {
	switch value {
	case "", PlacementPath, PlacementQuery, PlacementHeader, PlacementCookie:
		return PlacementPath, nil
	default:
		return "", E.New("unsupported xhttp ", name, ": ", value)
	}
}

func normalizeDataPlacement(value string) (string, error) {
	switch value {
	case "":
		return PlacementBody, nil
	case PlacementHeader, PlacementCookie, PlacementBody:
		return value, nil
	default:
		return "", E.New("unsupported xhttp uplink data placement: ", value)
	}
}
