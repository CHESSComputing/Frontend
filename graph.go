package main

import (
	"fmt"
	"strings"

	services "github.com/CHESSComputing/golib/services"
	"github.com/CHESSComputing/golib/utils"
)

// ---- cytoscape.js element shapes ----

type NodeData struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Group   string         `json:"group"`   // drives color-coding (schema name)
	Details map[string]any `json:"details"` // full record, shown on click
}

type GraphNode struct {
	Data NodeData `json:"data"`
}

type EdgeData struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphEdge struct {
	Data EdgeData `json:"data"`
}

type GraphElements struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// toStringSlice safely reads a []any or []string field from
// a decoded JSON map, tolerating records where the field is missing,
// null, or an empty string (as in the ID1A3 example's doi_* fields).
func toStringSlice(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = t
	}
	return out
}

// shortLabel picks a compact, human-readable node label instead of
// the full did path.
func shortLabel(r map[string]any, group string) string {
	doi, _ := r["doi"].(string)
	if doi != "" {
		return fmt.Sprintf("doi (%s)", doi)
	}
	app, _ := r["application"].(string)
	if app != "" {
		return fmt.Sprintf("metadata (%s)", app)
	}
	schema, _ := r["schema"].(string)
	if schema == "specscans" {
		return group
	}
	if schema != "" {
		return fmt.Sprintf("metadata (%s)", schema)
	}
	return group
}

// ownerDidKey tags a provenance/specscans record with the did of the
// metadata record it belongs to, so buildGraph can wire them together
// directly instead of guessing from the record's own "did" field.
const ownerDidKey = "_owner_did"

// fetchOwned fetches the metadata record for did, plus its provenance
// and specscans, tagging the latter two with the owning metadata did.
func fetchOwned(did string, records *[]map[string]any) {
	mrec, err := findMetadataRecord(did)
	if err != nil {
		return
	}
	*records = append(*records, mrec)

	mdid, _ := mrec["did"].(string)
	if mdid == "" {
		mdid = did
	}

	if provRecords, err := getData("provenance", mdid); err == nil {
		for _, p := range provRecords {
			p[ownerDidKey] = mdid
			*records = append(*records, p)
		}
	}

	spec := map[string]any{"did": mdid}
	if specRecords, err := fetchSpecScans(services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{Spec: spec},
	}); err == nil {
		for _, s := range specRecords {
			if _, ok := s["schema"]; !ok {
				s["schema"] = "specscans"
			}
			s[ownerDidKey] = mdid
			*records = append(*records, s)
		}
	}
}

func fetchGraphRecords(did string) []map[string]any {
	var records []map[string]any

	fetchOwned(did, &records)

	for _, r := range utils.List2Set(getParents(did)) {
		fetchOwned(r, &records)
	}

	var parents []string
	if provParents, err := getData("parents", did); err == nil {
		for _, r := range provParents {
			if f, ok := r["parent_did"]; ok && f != nil {
				if v, ok := f.(string); ok {
					parents = append(parents, v)
				}
			}
		}
	}
	for _, r := range utils.List2Set(parents) {
		fetchOwned(r, &records)
	}

	return records
}

func buildGraph(records []map[string]any) GraphElements {
	var els GraphElements

	nodeIDs := make([]string, len(records))
	groups := make([]string, len(records))
	var provIdx, specIdx int

	// Pass 1: assign each record its canonical node ID + group exactly
	// once. Everything downstream indexes into this slice rather than
	// recomputing IDs, so the two sides can never disagree.
	for i, r := range records {
		did, _ := r["did"].(string)
		if did == "" {
			continue // skip malformed records
		}
		schema, _ := r["schema"].(string)
		switch schema {
		case "specscans":
			groups[i] = "specscans"
			nodeIDs[i] = fmt.Sprintf("/specscans-%d%s", specIdx, did)
			specIdx++
		case "", "provenance":
			groups[i] = fmt.Sprintf("provenance-%d", provIdx)
			nodeIDs[i] = fmt.Sprintf("/provenance-%d%s", provIdx, did)
			provIdx++
		default:
			groups[i] = schema
			nodeIDs[i] = fmt.Sprintf("/record%s", did)
		}
	}

	// Pass 2: nodes, de-duplicated (a record can be pulled in via more
	// than one path — e.g. as both a parent and a provenance owner).
	seen := make(map[string]bool)
	for i, r := range records {
		if nodeIDs[i] == "" || seen[nodeIDs[i]] {
			continue
		}
		seen[nodeIDs[i]] = true
		els.Nodes = append(els.Nodes, GraphNode{Data: NodeData{
			ID:      nodeIDs[i],
			Label:   shortLabel(r, groups[i]),
			Group:   groups[i],
			Details: r,
		}})
	}

	// metadata did -> its node id, used both for parent_dids lineage
	// edges and for wiring provenance/specscans to their owner.
	metaNodeByDid := make(map[string]string)
	for i, r := range records {
		if strings.HasPrefix(nodeIDs[i], "/record") {
			did, _ := r["did"].(string)
			metaNodeByDid[did] = nodeIDs[i]
		}
	}

	addEdge := func(source, target string) {
		if source == "" || target == "" || source == target || !seen[source] || !seen[target] {
			return
		}
		els.Edges = append(els.Edges, GraphEdge{Data: EdgeData{
			ID: source + "->" + target, Source: source, Target: target,
		}})
	}

	for i, r := range records {
		nodeID := nodeIDs[i]
		if nodeID == "" {
			continue
		}
		// metadata -> metadata lineage
		for _, parentDid := range toStringSlice(r["parent_dids"]) {
			addEdge(metaNodeByDid[parentDid], nodeID)
		}
		if parentDid, ok := r["parent_did"].(string); ok && parentDid != "" {
			addEdge(metaNodeByDid[parentDid], nodeID)
		}
		// provenance / specscans -> the metadata record they belong to
		if ownerDid, ok := r[ownerDidKey].(string); ok && ownerDid != "" {
			addEdge(nodeID, metaNodeByDid[ownerDid])
		}
	}

	return els
}
