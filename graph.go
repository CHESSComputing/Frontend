package main

import (
	"fmt"
	"log"

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
	}

	// loop over parents records and append them to our list of records
	for _, r := range utils.List2Set(getParents(did)) {
		if mrec, err := findMetadataRecord(r); err == nil {
			records = append(records, mrec)
		}
	}

	// get children from provenance service
	var parents []string
	provParents, err := getData("parents", did)
	if err != nil {
		log.Printf("WARNING: error while fetching parents for did=%s, error=%v", did, err)
	}
	for _, r := range provParents {
		if f, ok := r["parent_did"]; ok {
			if f != nil {
				v := f.(string)
				parents = append(parents, v)
			}
		}
	}
	// ensure that children is unique list
	parents = utils.List2Set(parents)
	for _, r := range parents {
		if mrec, err := findMetadataRecord(r); err == nil {
			records = append(records, mrec)
		}
	}

	// loop over children records
	for _, r := range utils.List2Set(getChildren(did)) {
		if mrec, err := findMetadataRecord(r); err == nil {
			records = append(records, mrec)
		}
	}

	// get children from provenance service
	var children []string
	provChildren, err := getData("child", did)
	if err != nil {
		log.Printf("WARNING: error while fetching children for did=%s, error=%v", did, err)
	}
	for _, r := range provChildren {
		if f, ok := r["child_did"]; ok {
			if f != nil {
				v := f.(string)
				children = append(children, v)
			}
		}
	}
	// ensure that children is unique list
	children = utils.List2Set(children)
	for _, r := range children {
		if mrec, err := findMetadataRecord(r); err == nil {
			records = append(records, mrec)
		}
	}
	return records
}

// buildGraph turns a flat list of records into cytoscape.js elements.
// Each record's "did" becomes a node id; each entry in "parent_dids"
// becomes a directed edge parent -> child.
func buildGraph(records []map[string]any) GraphElements {
	var els GraphElements

	for _, r := range records {
		did, _ := r["did"].(string)
		if did == "" {
			continue // skip malformed records
		}

		schema, _ := r["schema"].(string)
		if schema == "" {
			schema = "Unknown"
		}

		els.Nodes = append(els.Nodes, GraphNode{Data: NodeData{
			ID:      did,
			Label:   shortLabel(r, did),
			Group:   schema,
			Details: r,
		}})

		for _, parent := range toStringSlice(r["parent_dids"]) {
			if parent == "" {
				continue
			}
			els.Edges = append(els.Edges, GraphEdge{Data: EdgeData{
				ID:     parent + "->" + did,
				Source: parent,
				Target: did,
			}})
		}
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
func shortLabel(r map[string]any, did string) string {
	doi, _ := r["doi"].(string)
	if doi != "" {
		return doi
	}
	label := "(raw)"
	schema, _ := r["schema"].(string)
	if schema != "" {
		label = fmt.Sprintf("metadata (%s)", schema)
	}
	app, _ := r["application"].(string)
	if app != "" {
		label = fmt.Sprintf("metadata (%s)", app)
	}
	return label
}
