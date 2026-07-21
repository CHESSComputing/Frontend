package main

import (
	"fmt"
	"regexp"
	"strings"

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

// fetchGraphRecords is a placeholder data source. Replace this with a real
// call to your FOXDEN MetaData/DataDiscovery API — the rest of the
// pipeline (buildGraph, template rendering) doesn't need to change,
// as long as each returned record has "did", "parent_dids" and
// (ideally) "schema".
func fetchGraphRecords(did string) []map[string]any {
	// use did:/beamline=3a/btr=test-demo/cycle=2026-1/sample_name=test-demo/test=Composite
	// for testing
	var records []map[string]any

	if mrec, err := findMetadataRecord(did); err == nil {
		records = append(records, mrec)
		mdid, _ := mrec["did"].(string)
		if provRecords, err := getData("provenance", mdid); err == nil {
			records = append(records, provRecords...)
		}
	}

	// loop over parents records and append them to our list of records
	for _, r := range utils.List2Set(getParents(did)) {
		if mrec, err := findMetadataRecord(r); err == nil {
			records = append(records, mrec)
			mdid, _ := mrec["did"].(string)
			if provRecords, err := getData("provenance", mdid); err == nil {
				records = append(records, provRecords...)
			}
		}
	}

	// get provenance record for our did
	if provRecords, err := getData("provenance", did); err == nil {
		records = append(records, provRecords...)
	}

	// get children from provenance service
	var parents []string
	provParents, _ := getData("parents", did)
	for _, r := range provParents {
		if f, ok := r["parent_did"]; ok {
			if f != nil {
				v := f.(string)
				parents = append(parents, v)
			}
		}
	}
	// ensure that parents is unique list
	parents = utils.List2Set(parents)
	for _, r := range parents {
		if mrec, err := findMetadataRecord(r); err == nil {
			records = append(records, mrec)
			provRecords, _ := getData("provenance", did)
			records = append(records, provRecords...)
		}
	}
	return records
}

// buildGraph turns a flat list of records into cytoscape.js elements.
// Each record's "did" becomes a node id; each entry in "parent_dids"
// becomes a directed edge parent -> child.
func buildGraph(records []map[string]any) GraphElements {
	var re = regexp.MustCompile(`^/provenance-[^/]+`)
	var els GraphElements

	var provIdx int
	var nodeids []string
	for _, r := range records {
		did, _ := r["did"].(string)
		if did == "" {
			continue // skip malformed records
		}

		schema, _ := r["schema"].(string)
		group := schema
		nodeID := fmt.Sprintf("/record%s", did)
		if group == "" {
			group = fmt.Sprintf("provenance-%d", provIdx)
			nodeID = fmt.Sprintf("/provenance-%d%s", provIdx, did)
			provIdx += 1
		}
		nodeids = append(nodeids, nodeID)
		label := shortLabel(r, group)

		els.Nodes = append(els.Nodes, GraphNode{Data: NodeData{
			ID:      nodeID,
			Label:   label,
			Group:   group,
			Details: r,
		}})

	}

	provIdx = 0
	for _, r := range records {
		did, _ := r["did"].(string)
		if did == "" {
			continue // skip malformed records
		}

		// create nodeID from record did
		schema, _ := r["schema"].(string)
		group := schema
		nodeID := fmt.Sprintf("/record%s", did)
		if group == "" {
			nodeID = fmt.Sprintf("/provenance-%d%s", provIdx, did)
			provIdx += 1
		}

		for _, parentDid := range toStringSlice(r["parent_dids"]) {
			if parentDid == "" {
				continue
			}
			var parentNodeID string
			schema, _ := r["schema"].(string)
			if schema != "" {
				parentNodeID = fmt.Sprintf("/record%s", parentDid)
				if utils.InList(parentNodeID, nodeids) {
					els.Edges = append(els.Edges, GraphEdge{Data: EdgeData{
						ID:     parentDid + "->" + nodeID,
						Source: parentNodeID,
						Target: nodeID,
					}})
				}
			}
		}
		// if record only contain parent_did (provenance records)
		if parentVal, ok := r["parent_did"]; ok {
			parentDid := parentVal.(string)
			var parentNodeID string
			schema, _ := r["schema"].(string)
			if schema != "" {
				parentNodeID = fmt.Sprintf("/record%s", parentDid)
				if parentDid != "" {
					els.Edges = append(els.Edges, GraphEdge{Data: EdgeData{
						ID:     parentDid + "->" + nodeID,
						Source: parentNodeID,
						Target: nodeID,
					}})
				}
			}
		}
	}

	// add provenance edges
	for _, nodeID := range nodeids {
		if !strings.HasPrefix(nodeID, "/provenance") {
			continue
		}
		// check out provenance edges
		recordID := re.ReplaceAllString(nodeID, "/record")
		els.Edges = append(els.Edges, GraphEdge{Data: EdgeData{
			ID:     nodeID + "->" + recordID,
			Source: nodeID,
			Target: recordID,
		}})
	}
	return els
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
	schema, _ := r["schema"].(string)
	if schema != "" {
		return fmt.Sprintf("metadata (%s)", schema)
	}
	app, _ := r["application"].(string)
	if app != "" {
		return fmt.Sprintf("metadata (%s)", app)
	}
	return group
}
