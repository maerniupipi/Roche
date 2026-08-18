package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	"roche.local/knowledge-agent-platform/internal/utils"
)

type graphConfigSummary struct {
	Nodes     []string
	Relations []string
}

var queryKnowledgeGraphTool = BaseTool{
	name: ToolQueryKnowledgeGraph,
	description: `Query knowledge graph to explore entity relationships and knowledge networks.

## Core Function
Explores relationships between entities in knowledge bases that have graph extraction configured.

## When to Use
✅ **Use for**:
- Understanding relationships between entities (e.g., "relationship between Docker and Kubernetes")
- Exploring knowledge networks and concept associations
- Finding related information about specific entities
- Understanding technical architecture and system relationships

❌ **Don't use for**:
- General text search → use knowledge_search
- Knowledge base without graph extraction configured
- Need exact document content → use knowledge_search

## Parameters
- **knowledge_base_ids** (required): Array of knowledge base IDs (1-10). Only KBs with graph extraction configured will be effective.
- **query** (required): Query content - can be entity name, relationship query, or concept search.

## Graph Configuration
Knowledge graph must be pre-configured in knowledge bases:
- **Entity types** (Nodes): e.g., "Technology", "Tool", "Concept"
- **Relationship types** (Relations): e.g., "depends_on", "uses", "contains"

If KB is not configured with graph, tool will return regular search results.

## Workflow
1. **Relationship exploration**: query_knowledge_graph → list_knowledge_chunks (for detailed content)
2. **Network analysis**: query_knowledge_graph → knowledge_search (for comprehensive understanding)
3. **Topic research**: knowledge_search → query_knowledge_graph (for deep entity relationships)

## Notes
- Results indicate graph configuration status
- Cross-KB results are automatically deduplicated
- Results are sorted by relevance`,
	schema: utils.GenerateSchema[QueryKnowledgeGraphInput](),
}

// QueryKnowledgeGraphInput defines the input parameters for query knowledge graph tool
type QueryKnowledgeGraphInput struct {
	KnowledgeBaseIDs []string `json:"knowledge_base_ids" jsonschema:"Array of knowledge base IDs to query"`
	Query            string   `json:"query" jsonschema:"Query content (entity name or query text)"`
}

// QueryKnowledgeGraphTool queries the knowledge graph for entities and relationships
type QueryKnowledgeGraphTool struct {
	BaseTool
	knowledgeService interfaces.KnowledgeBaseService
	graphRepository  interfaces.RetrieveGraphRepository
	searchTargets    types.SearchTargets
	enforceScope     bool
}

// NewQueryKnowledgeGraphTool creates a new query knowledge graph tool
func NewQueryKnowledgeGraphTool(knowledgeService interfaces.KnowledgeBaseService) *QueryKnowledgeGraphTool {
	return &QueryKnowledgeGraphTool{
		BaseTool:         queryKnowledgeGraphTool,
		knowledgeService: knowledgeService,
	}
}

func NewScopedQueryKnowledgeGraphTool(
	knowledgeService interfaces.KnowledgeBaseService,
	graphRepository interfaces.RetrieveGraphRepository,
	searchTargets types.SearchTargets,
) *QueryKnowledgeGraphTool {
	return &QueryKnowledgeGraphTool{
		BaseTool:         queryKnowledgeGraphTool,
		knowledgeService: knowledgeService,
		graphRepository:  graphRepository,
		searchTargets:    searchTargets,
		enforceScope:     true,
	}
}

// Execute performs the knowledge graph query with concurrent KB processing
func (t *QueryKnowledgeGraphTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	// Parse args from json.RawMessage
	var input QueryKnowledgeGraphInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse args: %v", err),
		}, err
	}

	// Extract knowledge_base_ids array
	if len(input.KnowledgeBaseIDs) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "knowledge_base_ids is required and must be a non-empty array",
		}, fmt.Errorf("knowledge_base_ids is required")
	}

	// Validate max 10 KBs
	if len(input.KnowledgeBaseIDs) > 10 {
		return &types.ToolResult{
			Success: false,
			Error:   "knowledge_base_ids must contain at most 10 KB IDs",
		}, fmt.Errorf("too many KB IDs")
	}
	knowledgeIDsByKB := make(map[string][]string, len(input.KnowledgeBaseIDs))
	if t.enforceScope {
		for _, kbID := range input.KnowledgeBaseIDs {
			allowed, fullAccess, knowledgeIDs := searchTargetsKnowledgeScope(t.searchTargets, kbID)
			if !allowed || (!fullAccess && len(knowledgeIDs) == 0) {
				err := fmt.Errorf("knowledge base %s is outside the current user's access scope", kbID)
				return &types.ToolResult{
					Success: false,
					Error:   err.Error(),
				}, err
			}
			if !fullAccess {
				knowledgeIDsByKB[kbID] = knowledgeIDs
			}
		}
	}

	query := input.Query
	if query == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "query is required",
		}, fmt.Errorf("invalid query")
	}

	// Concurrently query all knowledge bases
	type graphQueryResult struct {
		kbID    string
		kb      *types.KnowledgeBase
		results []*types.SearchResult
		graph   *types.GraphData
		err     error
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	kbResults := make(map[string]*graphQueryResult)
	graphSemaphore := make(chan struct{}, 8)

	for _, kbID := range input.KnowledgeBaseIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Get knowledge base to check graph configuration
			kb, err := t.knowledgeService.GetKnowledgeBaseByID(ctx, id)
			if err != nil {
				mu.Lock()
				kbResults[id] = &graphQueryResult{kbID: id, err: fmt.Errorf("failed to get knowledge base: %v", err)}
				mu.Unlock()
				return
			}

			// Check if graph extraction is enabled
			if kb.ExtractConfig == nil || (len(kb.ExtractConfig.Nodes) == 0 && len(kb.ExtractConfig.Relations) == 0) {
				mu.Lock()
				kbResults[id] = &graphQueryResult{kbID: id, err: fmt.Errorf("graph extraction not configured")}
				mu.Unlock()
				return
			}

			if t.graphRepository != nil {
				graph, graphErr := t.queryGraph(
					ctx,
					id,
					knowledgeIDsByKB[id],
					query,
					graphSemaphore,
				)
				mu.Lock()
				if graphErr != nil {
					kbResults[id] = &graphQueryResult{
						kbID: id,
						kb:   kb,
						err:  fmt.Errorf("graph query failed: %v", graphErr),
					}
				} else {
					kbResults[id] = &graphQueryResult{kbID: id, kb: kb, graph: graph}
				}
				mu.Unlock()
				return
			}

			// Compatibility fallback for callers that do not provide a graph
			// repository. Production agent wiring always provides Neo4j.
			// Query only the documents authorized for this user. Whole-KB grants
			// leave KnowledgeIDs empty; document grants carry the exact IDs.
			searchParams := types.SearchParams{
				QueryText:    query,
				MatchCount:   10,
				KnowledgeIDs: knowledgeIDsByKB[id],
			}
			results, err := t.knowledgeService.HybridSearch(ctx, id, searchParams)
			if err != nil {
				mu.Lock()
				kbResults[id] = &graphQueryResult{kbID: id, kb: kb, err: fmt.Errorf("query failed: %v", err)}
				mu.Unlock()
				return
			}

			mu.Lock()
			kbResults[id] = &graphQueryResult{kbID: id, kb: kb, results: results}
			mu.Unlock()
		}(kbID)
	}

	wg.Wait()

	// Collect and deduplicate results
	seenChunks := make(map[string]*types.SearchResult)
	var errors []string
	graphConfigs := make(map[string]graphConfigSummary)
	kbCounts := make(map[string]int)
	var graphNodes []*types.GraphNode
	var graphRelations []*types.GraphRelation

	for _, kbID := range input.KnowledgeBaseIDs {
		result := kbResults[kbID]
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("KB %s: %v", kbID, result.err))
			continue
		}

		if result.kb != nil && result.kb.ExtractConfig != nil {
			graphConfigs[kbID] = summarizeGraphConfig(result.kb.ExtractConfig)
		}

		if result.graph != nil {
			graphNodes = append(graphNodes, result.graph.Node...)
			graphRelations = append(graphRelations, result.graph.Relation...)
			kbCounts[kbID] = len(result.graph.Node)
			continue
		}

		kbCounts[kbID] = len(result.results)
		for _, r := range result.results {
			if _, seen := seenChunks[r.ID]; !seen {
				seenChunks[r.ID] = r
			}
		}
	}

	if t.graphRepository != nil {
		return buildDirectGraphToolResult(
			input.KnowledgeBaseIDs,
			query,
			graphNodes,
			graphRelations,
			graphConfigs,
			kbCounts,
			errors,
		), nil
	}

	// Convert map to slice and sort by score
	allResults := make([]*types.SearchResult, 0, len(seenChunks))
	for _, result := range seenChunks {
		allResults = append(allResults, result)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	if len(allResults) == 0 {
		return &types.ToolResult{
			Success: true,
			Output:  "No relevant graph information found.",
			Data: map[string]interface{}{
				"knowledge_base_ids": input.KnowledgeBaseIDs,
				"query":              query,
				"results":            []interface{}{},
				"graph_configs":      graphConfigsToData(graphConfigs),
				"graph_config":       aggregateGraphConfig(graphConfigs),
				"errors":             errors,
			},
		}, nil
	}

	// Format output with enhanced graph information
	output := "=== Knowledge Graph Query ===\n\n"
	output += fmt.Sprintf("📊 Query: %s\n", query)
	output += fmt.Sprintf("🎯 Target Knowledge Bases: %v\n", input.KnowledgeBaseIDs)
	output += fmt.Sprintf("✓ Found %d relevant results (deduplicated)\n\n", len(allResults))

	if len(errors) > 0 {
		output += "=== ⚠️ Partial Failures ===\n"
		for _, errMsg := range errors {
			output += fmt.Sprintf("  - %s\n", errMsg)
		}
		output += "\n"
	}

	// Display graph configuration status
	hasGraphConfig := false
	output += "=== 📈 Graph Configuration Status ===\n\n"
	for kbID, config := range graphConfigs {
		hasGraphConfig = true
		output += fmt.Sprintf("Knowledge Base [%s]:\n", kbID)

		if len(config.Nodes) > 0 {
			output += fmt.Sprintf("  ✓ Entity Types (%d): %v\n", len(config.Nodes), config.Nodes)
		} else {
			output += "  ⚠️ No entity types configured\n"
		}

		if len(config.Relations) > 0 {
			output += fmt.Sprintf("  ✓ Relationship Types (%d): %v\n", len(config.Relations), config.Relations)
		} else {
			output += "  ⚠️ No relationship types configured\n"
		}
		output += "\n"
	}

	if !hasGraphConfig {
		output += "⚠️ None of the queried knowledge bases have graph extraction configured\n"
		output += "💡 Hint: Configure entity and relationship types in knowledge base settings\n\n"
	}

	// Display result counts by KB
	if len(kbCounts) > 0 {
		output += "=== 📚 Knowledge Base Coverage ===\n"
		for kbID, count := range kbCounts {
			output += fmt.Sprintf("  - %s: %d results\n", kbID, count)
		}
		output += "\n"
	}

	// Display search results
	output += "=== 🔍 Query Results ===\n\n"
	if !hasGraphConfig {
		output += "💡 Returning relevant document chunks (knowledge base has no graph configuration)\n\n"
	} else {
		output += "💡 Content retrieval based on graph configuration\n\n"
	}

	formattedResults := make([]map[string]interface{}, 0, len(allResults))
	currentKB := ""

	for i, result := range allResults {
		// Group by knowledge base
		if result.KnowledgeID != currentKB {
			currentKB = result.KnowledgeID
			if i > 0 {
				output += "\n"
			}
			output += fmt.Sprintf("[Source Document: %s]\n\n", result.KnowledgeTitle)
		}

		relevanceLevel := GetRelevanceLevel(result.Score)

		output += fmt.Sprintf("Result #%d:\n", i+1)
		output += fmt.Sprintf("  📍 Relevance: %.2f (%s)\n", result.Score, relevanceLevel)
		output += fmt.Sprintf("  🔗 Match Type: %s\n", FormatMatchType(result.MatchType))
		output += fmt.Sprintf("  📄 Content: %s\n", result.Content)
		output += fmt.Sprintf("  🆔 chunk_id: %s\n\n", result.ID)

		formattedResults = append(formattedResults, map[string]interface{}{
			"result_index":    i + 1,
			"chunk_id":        result.ID,
			"content":         result.Content,
			"score":           result.Score,
			"relevance_level": relevanceLevel,
			"knowledge_id":    result.KnowledgeID,
			"knowledge_title": result.KnowledgeTitle,
			"match_type":      FormatMatchType(result.MatchType),
		})
	}

	output += "=== 💡 Tips ===\n"
	output += "- ✓ Results are deduplicated across knowledge bases and sorted by relevance\n"
	output += "- ✓ Use get_chunk_detail to get full content\n"
	output += "- ✓ Use list_knowledge_chunks to explore context\n"
	if !hasGraphConfig {
		output += "- ⚠️ Configure graph extraction for more precise entity-relationship results\n"
	}
	output += "- ⏳ Full graph query language (Cypher) support is under development\n"

	// Build structured graph data for frontend visualization
	graphData := buildGraphVisualizationData(allResults)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"knowledge_base_ids": input.KnowledgeBaseIDs,
			"query":              query,
			"results":            formattedResults,
			"count":              len(allResults),
			"kb_counts":          kbCounts,
			"graph_configs":      graphConfigsToData(graphConfigs),
			"graph_config":       aggregateGraphConfig(graphConfigs),
			"graph_data":         graphData,
			"has_graph_config":   hasGraphConfig,
			"errors":             errors,
			"display_type":       "graph_query_results",
		},
	}, nil
}

func (t *QueryKnowledgeGraphTool) queryGraph(
	ctx context.Context,
	kbID string,
	knowledgeIDs []string,
	query string,
	semaphore chan struct{},
) (*types.GraphData, error) {
	nodes := graphQueryTerms(query)
	if len(knowledgeIDs) == 0 {
		semaphore <- struct{}{}
		defer func() { <-semaphore }()
		return t.graphRepository.SearchNode(ctx, types.NameSpace{KnowledgeBase: kbID}, nodes)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var allNodes []*types.GraphNode
	var allRelations []*types.GraphRelation
	var firstErr error

	for _, knowledgeID := range knowledgeIDs {
		knowledgeID := knowledgeID
		wg.Add(1)
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			graph, err := t.graphRepository.SearchNode(ctx, types.NameSpace{
				KnowledgeBase: kbID,
				Knowledge:     knowledgeID,
			}, nodes)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if graph != nil {
				allNodes = append(allNodes, graph.Node...)
				allRelations = append(allRelations, graph.Relation...)
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return mergeGraphData(allNodes, allRelations), nil
}

func graphQueryTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	terms := []string{query}
	for _, part := range strings.FieldsFunc(query, func(r rune) bool {
		switch r {
		case ' ', '\t', '\r', '\n', ',', '，', ';', '；', ':', '：', '/', '\\', '|':
			return true
		default:
			return false
		}
	}) {
		part = strings.TrimSpace(part)
		if part != "" && part != query {
			terms = append(terms, part)
		}
	}
	return uniqueStrings(terms)
}

func mergeGraphData(
	nodes []*types.GraphNode,
	relations []*types.GraphRelation,
) *types.GraphData {
	nodeByName := make(map[string]*types.GraphNode)
	for _, node := range nodes {
		if node == nil || node.Name == "" {
			continue
		}
		current := nodeByName[node.Name]
		if current == nil {
			current = &types.GraphNode{Name: node.Name}
			nodeByName[node.Name] = current
		}
		current.Chunks = append(current.Chunks, node.Chunks...)
		current.Attributes = append(current.Attributes, node.Attributes...)
	}

	mergedNodes := make([]*types.GraphNode, 0, len(nodeByName))
	for _, node := range nodeByName {
		node.Chunks = uniqueStrings(node.Chunks)
		node.Attributes = uniqueStrings(node.Attributes)
		mergedNodes = append(mergedNodes, node)
	}
	sort.Slice(mergedNodes, func(i, j int) bool {
		return mergedNodes[i].Name < mergedNodes[j].Name
	})

	relationSeen := make(map[string]bool)
	mergedRelations := make([]*types.GraphRelation, 0, len(relations))
	for _, relation := range relations {
		if relation == nil {
			continue
		}
		key := relation.Node1 + "\x00" + relation.Type + "\x00" + relation.Node2
		if relationSeen[key] {
			continue
		}
		relationSeen[key] = true
		mergedRelations = append(mergedRelations, relation)
	}
	sort.Slice(mergedRelations, func(i, j int) bool {
		left := mergedRelations[i].Node1 + mergedRelations[i].Type + mergedRelations[i].Node2
		right := mergedRelations[j].Node1 + mergedRelations[j].Type + mergedRelations[j].Node2
		return left < right
	})

	return &types.GraphData{Node: mergedNodes, Relation: mergedRelations}
}

func buildDirectGraphToolResult(
	kbIDs []string,
	query string,
	nodes []*types.GraphNode,
	relations []*types.GraphRelation,
	graphConfigs map[string]graphConfigSummary,
	kbCounts map[string]int,
	errors []string,
) *types.ToolResult {
	graph := mergeGraphData(nodes, relations)
	chunkIDs := make([]string, 0)
	for _, node := range graph.Node {
		chunkIDs = append(chunkIDs, node.Chunks...)
	}
	chunkIDs = uniqueStrings(chunkIDs)

	var output strings.Builder
	output.WriteString("=== Knowledge Graph Query ===\n\n")
	output.WriteString(fmt.Sprintf("Query: %s\n", query))
	output.WriteString(fmt.Sprintf("Target Knowledge Bases: %v\n", kbIDs))
	output.WriteString(fmt.Sprintf("Found %d entities and %d relationships.\n\n", len(graph.Node), len(graph.Relation)))

	if len(errors) > 0 {
		output.WriteString("Partial failures:\n")
		for _, errMessage := range errors {
			output.WriteString(fmt.Sprintf("- %s\n", errMessage))
		}
		output.WriteString("\n")
	}

	if len(graph.Node) > 0 {
		output.WriteString("Entities:\n")
		for _, node := range graph.Node {
			output.WriteString(fmt.Sprintf("- %s", node.Name))
			if len(node.Attributes) > 0 {
				output.WriteString(fmt.Sprintf(" | attributes: %v", node.Attributes))
			}
			if len(node.Chunks) > 0 {
				output.WriteString(fmt.Sprintf(" | source_chunk_ids: %v", node.Chunks))
			}
			output.WriteString("\n")
		}
		output.WriteString("\n")
	}

	if len(graph.Relation) > 0 {
		output.WriteString("Relationships:\n")
		for _, relation := range graph.Relation {
			output.WriteString(fmt.Sprintf("- %s --[%s]--> %s\n", relation.Node1, relation.Type, relation.Node2))
		}
		output.WriteString("\n")
	}

	if len(graph.Node) == 0 && len(graph.Relation) == 0 {
		output.WriteString("No relevant graph information found.\n")
	}
	if len(chunkIDs) > 0 {
		output.WriteString(fmt.Sprintf("Use list_knowledge_chunks with these source chunk IDs for full evidence: %v\n", chunkIDs))
	}

	return &types.ToolResult{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"knowledge_base_ids": kbIDs,
			"query":              query,
			"graph_configs":      graphConfigsToData(graphConfigs),
			"graph_config":       aggregateGraphConfig(graphConfigs),
			"graph_data":         graphDataToVisualization(graph),
			"chunk_ids":          chunkIDs,
			"entity_count":       len(graph.Node),
			"relation_count":     len(graph.Relation),
			"kb_counts":          kbCounts,
			"errors":             errors,
			"display_type":       "graph_query_results",
		},
	}
}

func graphDataToVisualization(graph *types.GraphData) map[string]interface{} {
	nodes := make([]map[string]interface{}, 0, len(graph.Node))
	for _, node := range graph.Node {
		nodes = append(nodes, map[string]interface{}{
			"id":         node.Name,
			"label":      node.Name,
			"type":       "entity",
			"chunks":     node.Chunks,
			"attributes": node.Attributes,
		})
	}
	edges := make([]map[string]interface{}, 0, len(graph.Relation))
	for i, relation := range graph.Relation {
		edges = append(edges, map[string]interface{}{
			"id":     fmt.Sprintf("edge-%d", i+1),
			"source": relation.Node1,
			"target": relation.Node2,
			"label":  relation.Type,
		})
	}
	return map[string]interface{}{
		"nodes":       nodes,
		"edges":       edges,
		"total_nodes": len(nodes),
		"total_edges": len(edges),
	}
}

func summarizeGraphConfig(config *types.ExtractConfig) graphConfigSummary {
	if config == nil {
		return graphConfigSummary{}
	}

	return graphConfigSummary{
		Nodes:     uniqueSortedNodeNames(config.Nodes),
		Relations: uniqueSortedRelationNames(config.Relations),
	}
}

func uniqueSortedNodeNames(nodes []*types.GraphNode) []string {
	seen := make(map[string]struct{}, len(nodes))
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || node.Name == "" {
			continue
		}
		if _, exists := seen[node.Name]; exists {
			continue
		}
		seen[node.Name] = struct{}{}
		names = append(names, node.Name)
	}
	sort.Strings(names)
	return names
}

func uniqueSortedRelationNames(relations []*types.GraphRelation) []string {
	seen := make(map[string]struct{}, len(relations))
	names := make([]string, 0, len(relations))
	for _, relation := range relations {
		if relation == nil || relation.Type == "" {
			continue
		}
		if _, exists := seen[relation.Type]; exists {
			continue
		}
		seen[relation.Type] = struct{}{}
		names = append(names, relation.Type)
	}
	sort.Strings(names)
	return names
}

func graphConfigsToData(graphConfigs map[string]graphConfigSummary) map[string]map[string]interface{} {
	if len(graphConfigs) == 0 {
		return nil
	}

	data := make(map[string]map[string]interface{}, len(graphConfigs))
	for kbID, config := range graphConfigs {
		data[kbID] = map[string]interface{}{
			"nodes":     config.Nodes,
			"relations": config.Relations,
		}
	}
	return data
}

func aggregateGraphConfig(graphConfigs map[string]graphConfigSummary) map[string]interface{} {
	if len(graphConfigs) == 0 {
		return nil
	}

	merged := graphConfigSummary{}
	for _, config := range graphConfigs {
		merged.Nodes = append(merged.Nodes, config.Nodes...)
		merged.Relations = append(merged.Relations, config.Relations...)
	}

	return map[string]interface{}{
		"nodes":     uniqueStrings(merged.Nodes),
		"relations": uniqueStrings(merged.Relations),
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// buildGraphVisualizationData builds structured data for graph visualization
func buildGraphVisualizationData(results []*types.SearchResult) map[string]interface{} {
	// Build a simple graph structure for frontend visualization
	nodes := make([]map[string]interface{}, 0)
	edges := make([]map[string]interface{}, 0)

	// Create nodes from results
	seenEntities := make(map[string]bool)
	for i, result := range results {
		if !seenEntities[result.ID] {
			nodes = append(nodes, map[string]interface{}{
				"id":       result.ID,
				"label":    fmt.Sprintf("Chunk %d", i+1),
				"content":  result.Content,
				"kb_id":    result.KnowledgeID,
				"kb_title": result.KnowledgeTitle,
				"score":    result.Score,
				"type":     "chunk",
			})
			seenEntities[result.ID] = true
		}
	}

	return map[string]interface{}{
		"nodes":       nodes,
		"edges":       edges,
		"total_nodes": len(nodes),
		"total_edges": len(edges),
	}
}
