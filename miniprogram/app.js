App({
  onLaunch() {
    const settings = wx.getStorageSync("roche_kap_settings");
    if (!settings) {
      wx.setStorageSync("roche_kap_settings", {
        baseUrl: "http://localhost:8080",
        apiKey: "",
        selectedKnowledgeBaseId: ""
      });
    }
  }
});
