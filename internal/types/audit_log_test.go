package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAuditAction_DotNamespaceConvention ensures every AuditAction
// constant follows the dot-namespaced `<area>.<event>` convention
// documented at the top of audit_log.go. Future PRs that add actions
// must keep this invariant so audit-log consumers can prefix-filter
// by area.
func TestAuditAction_DotNamespaceConvention(t *testing.T) {
	all := []AuditAction{
		AuditActionAccessDenied,
		// VectorStore namespace
		AuditActionVectorStoreCreated,
		AuditActionVectorStoreUpdated,
		AuditActionVectorStoreDeleted,
		// OpenSearch namespace
		AuditActionOpenSearchIndexCreated,
		AuditActionOpenSearchIndexDeleted,
		AuditActionOpenSearchReindexExecuted,
		// System namespace
		AuditActionSystemSettingChanged,
		AuditActionSystemAdminPromoted,
		AuditActionSystemAdminRevoked,
		// HTTP namespace
		AuditActionHTTPRequest,
		// Knowledge namespace
		AuditActionKnowledgeCreated,
		AuditActionKnowledgeUpdated,
		AuditActionKnowledgeDeleted,
		AuditActionKnowledgePublished,
		AuditActionKnowledgeUnpublished,
		// FAQ namespace
		AuditActionFAQCreated,
		AuditActionFAQUpdated,
		AuditActionFAQDeleted,
		AuditActionFAQImported,
		// KnowledgeBase namespace
		AuditActionKnowledgeBaseCreated,
		AuditActionKnowledgeBaseUpdated,
		AuditActionKnowledgeBaseDeleted,
		// Permission namespace
		AuditActionPermissionGranted,
		AuditActionPermissionRevoked,
		AuditActionDomainAdminGranted,
		AuditActionDomainAdminRevoked,
		// User namespace
		AuditActionUserCreated,
		AuditActionUserUpdated,
		AuditActionUserDeleted,
		AuditActionUserPasswordChanged,
		// Session / QA namespace
		AuditActionSessionCreated,
		AuditActionSessionDeleted,
		AuditActionQACompleted,
		// DataSource namespace
		AuditActionDataSourceCreated,
		AuditActionDataSourceUpdated,
		AuditActionDataSourceDeleted,
		// KnowledgeDomain namespace
		AuditActionKnowledgeDomainCreated,
		AuditActionKnowledgeDomainUpdated,
		AuditActionKnowledgeDomainDeleted,
	}
	for _, a := range all {
		s := string(a)
		area, event, ok := strings.Cut(s, ".")
		assert.True(t, ok,
			"action %q must contain exactly one dot separator", s)
		assert.NotEmpty(t, area, "action %q has empty area", s)
		assert.NotEmpty(t, event, "action %q has empty event", s)
	}
}

// TestAuditAction_VectorStoreNamespacePrefix pins the three
// vector_store.* actions to their shared area prefix.
func TestAuditAction_VectorStoreNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionVectorStoreCreated,
		AuditActionVectorStoreUpdated,
		AuditActionVectorStoreDeleted,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "向量库."),
			"expected %q to start with '向量库.'", a,
		)
	}
}

// TestAuditAction_OpenSearchNamespacePrefix pins the three
// opensearch.* actions to their shared area prefix.
func TestAuditAction_OpenSearchNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionOpenSearchIndexCreated,
		AuditActionOpenSearchIndexDeleted,
		AuditActionOpenSearchReindexExecuted,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "检索引擎."),
			"expected %q to start with '检索引擎.'", a,
		)
	}
}

// TestAuditAction_NoCollisionsAcrossNamespaces ensures no two
// AuditAction constants share the same wire string.
func TestAuditAction_NoCollisionsAcrossNamespaces(t *testing.T) {
	seen := make(map[AuditAction]string)
	register := func(name string, a AuditAction) {
		t.Helper()
		if prev, exists := seen[a]; exists {
			t.Fatalf("collision: %s and %s both map to %q", prev, name, a)
		}
		seen[a] = name
	}
	register("AuditActionAccessDenied", AuditActionAccessDenied)
	register("AuditActionVectorStoreCreated", AuditActionVectorStoreCreated)
	register("AuditActionVectorStoreUpdated", AuditActionVectorStoreUpdated)
	register("AuditActionVectorStoreDeleted", AuditActionVectorStoreDeleted)
	register("AuditActionOpenSearchIndexCreated", AuditActionOpenSearchIndexCreated)
	register("AuditActionOpenSearchIndexDeleted", AuditActionOpenSearchIndexDeleted)
	register("AuditActionOpenSearchReindexExecuted", AuditActionOpenSearchReindexExecuted)
	register("AuditActionSystemSettingChanged", AuditActionSystemSettingChanged)
	register("AuditActionSystemAdminPromoted", AuditActionSystemAdminPromoted)
	register("AuditActionSystemAdminRevoked", AuditActionSystemAdminRevoked)
	register("AuditActionHTTPRequest", AuditActionHTTPRequest)
	register("AuditActionKnowledgeCreated", AuditActionKnowledgeCreated)
	register("AuditActionKnowledgeUpdated", AuditActionKnowledgeUpdated)
	register("AuditActionKnowledgeDeleted", AuditActionKnowledgeDeleted)
	register("AuditActionKnowledgePublished", AuditActionKnowledgePublished)
	register("AuditActionKnowledgeUnpublished", AuditActionKnowledgeUnpublished)
	register("AuditActionFAQCreated", AuditActionFAQCreated)
	register("AuditActionFAQUpdated", AuditActionFAQUpdated)
	register("AuditActionFAQDeleted", AuditActionFAQDeleted)
	register("AuditActionFAQImported", AuditActionFAQImported)
	register("AuditActionKnowledgeBaseCreated", AuditActionKnowledgeBaseCreated)
	register("AuditActionKnowledgeBaseUpdated", AuditActionKnowledgeBaseUpdated)
	register("AuditActionKnowledgeBaseDeleted", AuditActionKnowledgeBaseDeleted)
	register("AuditActionPermissionGranted", AuditActionPermissionGranted)
	register("AuditActionPermissionRevoked", AuditActionPermissionRevoked)
	register("AuditActionDomainAdminGranted", AuditActionDomainAdminGranted)
	register("AuditActionDomainAdminRevoked", AuditActionDomainAdminRevoked)
	register("AuditActionUserCreated", AuditActionUserCreated)
	register("AuditActionUserUpdated", AuditActionUserUpdated)
	register("AuditActionUserDeleted", AuditActionUserDeleted)
	register("AuditActionUserPasswordChanged", AuditActionUserPasswordChanged)
	register("AuditActionSessionCreated", AuditActionSessionCreated)
	register("AuditActionSessionDeleted", AuditActionSessionDeleted)
	register("AuditActionQACompleted", AuditActionQACompleted)
	register("AuditActionDataSourceCreated", AuditActionDataSourceCreated)
	register("AuditActionDataSourceUpdated", AuditActionDataSourceUpdated)
	register("AuditActionDataSourceDeleted", AuditActionDataSourceDeleted)
	register("AuditActionKnowledgeDomainCreated", AuditActionKnowledgeDomainCreated)
	register("AuditActionKnowledgeDomainUpdated", AuditActionKnowledgeDomainUpdated)
	register("AuditActionKnowledgeDomainDeleted", AuditActionKnowledgeDomainDeleted)
}

// TestAuditAction_SystemNamespacePrefix pins the three system.* actions
// to their shared area prefix.
func TestAuditAction_SystemNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionSystemSettingChanged,
		AuditActionSystemAdminPromoted,
		AuditActionSystemAdminRevoked,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "系统."),
			"expected %q to start with '系统.'", a,
		)
	}
}

// TestAuditAction_SystemWireValues pins the exact wire strings for
// the three system.* actions.
func TestAuditAction_SystemWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionSystemSettingChanged, "系统.设置变更"},
		{AuditActionSystemAdminPromoted, "系统.管理员提升"},
		{AuditActionSystemAdminRevoked, "系统.管理员撤销"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_Phase3WireValues pins the exact wire strings for
// the six Phase 3 actions.
func TestAuditAction_Phase3WireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionVectorStoreCreated, "向量库.创建"},
		{AuditActionVectorStoreUpdated, "向量库.更新"},
		{AuditActionVectorStoreDeleted, "向量库.删除"},
		{AuditActionOpenSearchIndexCreated, "检索引擎.索引创建"},
		{AuditActionOpenSearchIndexDeleted, "检索引擎.索引删除"},
		{AuditActionOpenSearchReindexExecuted, "检索引擎.重建索引"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_KnowledgeWireValues pins the exact wire strings for
// knowledge document lifecycle actions.
func TestAuditAction_KnowledgeWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionKnowledgeCreated, "知识.创建"},
		{AuditActionKnowledgeUpdated, "知识.更新"},
		{AuditActionKnowledgeDeleted, "知识.删除"},
		{AuditActionKnowledgePublished, "知识.发布"},
		{AuditActionKnowledgeUnpublished, "知识.下架"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_FAQWireValues pins the exact wire strings for
// FAQ lifecycle actions.
func TestAuditAction_FAQWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionFAQCreated, "FAQ.创建"},
		{AuditActionFAQUpdated, "FAQ.更新"},
		{AuditActionFAQDeleted, "FAQ.删除"},
		{AuditActionFAQImported, "FAQ.导入"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_KnowledgeBaseWireValues pins the wire strings for
// knowledge base lifecycle actions.
func TestAuditAction_KnowledgeBaseWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionKnowledgeBaseCreated, "知识库.创建"},
		{AuditActionKnowledgeBaseUpdated, "知识库.更新"},
		{AuditActionKnowledgeBaseDeleted, "知识库.删除"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_PermissionWireValues pins the wire strings for
// permission-related actions.
func TestAuditAction_PermissionWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionPermissionGranted, "权限.授予"},
		{AuditActionPermissionRevoked, "权限.撤销"},
		{AuditActionDomainAdminGranted, "域.管理员授予"},
		{AuditActionDomainAdminRevoked, "域.管理员撤销"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_UserWireValues pins the wire strings for
// user management actions.
func TestAuditAction_UserWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionUserCreated, "用户.创建"},
		{AuditActionUserUpdated, "用户.更新"},
		{AuditActionUserDeleted, "用户.删除"},
		{AuditActionUserPasswordChanged, "用户.密码变更"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_SessionQAWireValues pins the wire strings for
// session and Q&A actions.
func TestAuditAction_SessionQAWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionSessionCreated, "会话.创建"},
		{AuditActionSessionDeleted, "会话.删除"},
		{AuditActionQACompleted, "问答.完成"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_DataSourceWireValues pins the wire strings for
// data source lifecycle actions.
func TestAuditAction_DataSourceWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionDataSourceCreated, "数据源.创建"},
		{AuditActionDataSourceUpdated, "数据源.更新"},
		{AuditActionDataSourceDeleted, "数据源.删除"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_KnowledgeDomainWireValues pins the wire strings for
// knowledge domain lifecycle actions.
func TestAuditAction_KnowledgeDomainWireValues(t *testing.T) {
	cases := []struct {
		constant AuditAction
		wire     string
	}{
		{AuditActionKnowledgeDomainCreated, "知识域.创建"},
		{AuditActionKnowledgeDomainUpdated, "知识域.更新"},
		{AuditActionKnowledgeDomainDeleted, "知识域.删除"},
	}
	for _, c := range cases {
		assert.Equal(t, c.wire, string(c.constant))
	}
}

// TestAuditAction_KnowledgeNamespacePrefix pins the five knowledge.* actions
// to their shared area prefix.
func TestAuditAction_KnowledgeNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionKnowledgeCreated,
		AuditActionKnowledgeUpdated,
		AuditActionKnowledgeDeleted,
		AuditActionKnowledgePublished,
		AuditActionKnowledgeUnpublished,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "知识."),
			"expected %q to start with '知识.'", a,
		)
	}
}

// TestAuditAction_FAQNamespacePrefix pins FAQ actions to their area.
func TestAuditAction_FAQNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionFAQCreated,
		AuditActionFAQUpdated,
		AuditActionFAQDeleted,
		AuditActionFAQImported,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "FAQ."),
			"expected %q to start with 'FAQ.'", a,
		)
	}
}

// TestAuditAction_PermissionNamespacePrefix pins permission actions to area.
func TestAuditAction_PermissionNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionPermissionGranted,
		AuditActionPermissionRevoked,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "权限."),
			"expected %q to start with '权限.'", a,
		)
	}
}

// TestAuditAction_DomainNamespacePrefix pins domain admin actions to area.
func TestAuditAction_DomainNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionDomainAdminGranted,
		AuditActionDomainAdminRevoked,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "域."),
			"expected %q to start with '域.'", a,
		)
	}
}

// TestAuditAction_UserNamespacePrefix pins user actions to their area.
func TestAuditAction_UserNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionUserCreated,
		AuditActionUserUpdated,
		AuditActionUserDeleted,
		AuditActionUserPasswordChanged,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "用户."),
			"expected %q to start with '用户.'", a,
		)
	}
}

// TestAuditAction_DataSourceNamespacePrefix pins datasource actions to area.
func TestAuditAction_DataSourceNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionDataSourceCreated,
		AuditActionDataSourceUpdated,
		AuditActionDataSourceDeleted,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "数据源."),
			"expected %q to start with '数据源.'", a,
		)
	}
}

// TestAuditAction_KnowledgeDomainNamespacePrefix pins knowledge_domain actions.
func TestAuditAction_KnowledgeDomainNamespacePrefix(t *testing.T) {
	cases := []AuditAction{
		AuditActionKnowledgeDomainCreated,
		AuditActionKnowledgeDomainUpdated,
		AuditActionKnowledgeDomainDeleted,
	}
	for _, a := range cases {
		assert.True(t,
			strings.HasPrefix(string(a), "知识域."),
			"expected %q to start with '知识域.'", a,
		)
	}
}
