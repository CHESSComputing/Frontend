package main

import (
	"fmt"
	"strings"

	services "github.com/CHESSComputing/golib/services"
	"github.com/CHESSComputing/golib/utils"
)

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
	ID      string `json:"id"`
	Source  string `json:"source"`
	Target  string `json:"target"`
	Prov    bool   `json:"prov,omitempty"`
	ProvDid string `json:"provDid,omitempty"`
}

type GraphEdge struct {
	Data EdgeData `json:"data"`
}

type GraphElements struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

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

const ownerDidKey = "_owner_did"

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
	provDidByOwner := make(map[string]string)

	var provIdx, specIdx int

	// Pass 1: Identify records and map provenance DIDs by owner metadata record
	for i, r := range records {
		did, _ := r["did"].(string)
		schema, _ := r["schema"].(string)
		ownerDid, hasOwner := r[ownerDidKey].(string)

		isSpec := (schema == "specscans")
		// Provenance records often have schema == "", so check ownerDidKey as well
		isProv := (schema == "provenance" || strings.HasPrefix(schema, "provenance") || (hasOwner && ownerDid != "" && !isSpec))

		if isSpec {
			groups[i] = "specscans"
			nodeIDs[i] = fmt.Sprintf("/specscans-%d%s", specIdx, did)
			specIdx++
		} else if isProv {
			provID := fmt.Sprintf("/provenance-%d%s", provIdx, did)
			provIdx++
			targetOwner := ownerDid
			if targetOwner == "" {
				targetOwner = did
			}
			if targetOwner != "" {
				provDidByOwner[targetOwner] = provID
			}
		} else {
			groups[i] = schema
			if did != "" {
				nodeIDs[i] = fmt.Sprintf("/record%s", did)
			}
		}
	}

	// Pass 2: Create nodes (provenance records are skipped from becoming nodes)
	seen := make(map[string]bool)
	for i, r := range records {
		nodeID := nodeIDs[i]
		schema, _ := r["schema"].(string)
		ownerDid, hasOwner := r[ownerDidKey].(string)
		isSpec := (schema == "specscans")
		isProv := (schema == "provenance" || strings.HasPrefix(schema, "provenance") || (hasOwner && ownerDid != "" && !isSpec))

		if isProv || nodeID == "" || seen[nodeID] {
			continue
		}
		seen[nodeID] = true

		did, _ := r["did"].(string)
		// Attach provenance DID to metadata details so frontend root-node logic can pick it up
		if provDid, ok := provDidByOwner[did]; ok {
			r["_prov_did"] = provDid
		}

		els.Nodes = append(els.Nodes, GraphNode{Data: NodeData{
			ID:      nodeID,
			Label:   shortLabel(r, groups[i]),
			Group:   groups[i],
			Details: r,
		}})
	}

	metaNodeByDid := make(map[string]string)
	for i, r := range records {
		if strings.HasPrefix(nodeIDs[i], "/record") {
			did, _ := r["did"].(string)
			if did != "" {
				metaNodeByDid[did] = nodeIDs[i]
			}
		}
	}

	addEdge := func(source, target string, provDid string) {
		if source == "" || target == "" || source == target || !seen[source] || !seen[target] {
			return
		}
		edge := GraphEdge{Data: EdgeData{
			ID:     source + "->" + target,
			Source: source,
			Target: target,
		}}
		if provDid != "" {
			edge.Data.Prov = true
			edge.Data.ProvDid = provDid
		}
		els.Edges = append(els.Edges, edge)
	}

	for i, r := range records {
		nodeID := nodeIDs[i]
		if nodeID == "" || !strings.HasPrefix(nodeID, "/record") {
			continue
		}

		did, _ := r["did"].(string)
		targetProvDid := provDidByOwner[did]

		// Metadata -> Metadata lineage (tagged with target node's provenance DID)
		for _, parentDid := range toStringSlice(r["parent_dids"]) {
			addEdge(metaNodeByDid[parentDid], nodeID, targetProvDid)
		}
		if parentDid, ok := r["parent_did"].(string); ok && parentDid != "" {
			addEdge(metaNodeByDid[parentDid], nodeID, targetProvDid)
		}

		// Specscans -> Metadata link
		if ownerDid, ok := r[ownerDidKey].(string); ok && ownerDid != "" {
			schema, _ := r["schema"].(string)
			if schema == "specscans" {
				addEdge(nodeID, metaNodeByDid[ownerDid], "")
			}
		}
	}

	return els
}
