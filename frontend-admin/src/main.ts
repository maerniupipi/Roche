import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import router from "./router";
import TDesign from "tdesign-vue-next";
// 引入组件库的少量全局样式变量
import "tdesign-vue-next/es/style/index.css";
import "@/assets/theme/theme.css";
import "@/assets/dropdown-menu.less";

import "@/assets/overrides/TDesign.less";
// vue-virtual-scroller ships its own tiny stylesheet — required for
// RecycleScroller/DynamicScroller to size their viewport correctly.
// Without it the scroller computes 0 height and renders no items.
import "vue-virtual-scroller/dist/vue-virtual-scroller.css";
import i18n from "./i18n";
import { initTheme } from "@/composables/useTheme";
import { initFont } from "@/composables/useFont";
import { installTDesignIconOfflineGuard } from "@/utils/tdesign-icon-offline";
import { installAutofillGuard } from "@/utils/disable-autofill";

// echarts 按需引入：仅注册仪表盘所需的渲染器与图表/组件/特性，
// 避免全量引入导致 bundle 膨胀（echarts 体积较大）。
import { use } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { PieChart, BarChart, LineChart } from "echarts/charts";
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  DatasetComponent,
  TransformComponent,
} from "echarts/components";
import VChart from "vue-echarts";

use([
  CanvasRenderer,
  PieChart,
  BarChart,
  LineChart,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  DatasetComponent,
  TransformComponent,
]);

// 必须在 Vue 组件挂载之前执行，避免 tdesign-icons 运行时请求 tdesign.gtimg.com
installTDesignIconOfflineGuard();

initTheme();
initFont();

const app = createApp(App);

// 全局错误处理：捕获未处理的组件错误，防止白屏
app.config.errorHandler = (err, instance, info) => {
  console.error("[RocheKAP] Unhandled Vue error:", err, "\nComponent:", instance, "\nInfo:", info);
};

app.use(TDesign);
app.use(createPinia());
app.use(router);
app.use(i18n);

// 全局注册 vue-echarts 子组件：仪表盘图表统一用 <v-chart> 渲染。
app.component("v-chart", VChart);

// 等首屏路由（含导航守卫）完成后再挂载，避免先闪默认页再跳转
router.isReady().finally(() => {
  app.mount("#app");
  installAutofillGuard();
});
