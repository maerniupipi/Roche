<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { mockDashboard } from "@/mock/dashboard";
import KnowledgeBaseStats from "./components/KnowledgeBaseStats.vue";
import AnswerBasicData from "./components/AnswerBasicData.vue";
import AnswerDetailData from "./components/AnswerDetailData.vue";
import DomainPieChart from "./components/DomainPieChart.vue";
import RingChart from "./components/RingChart.vue";
import TopUsersRanking from "./components/TopUsersRanking.vue";
import TopDocsRanking from "./components/TopDocsRanking.vue";
import FeedbackBarChartH from "./components/FeedbackBarChartH.vue";
import FallbackQuestionsList from "./components/FallbackQuestionsList.vue";
import SummaryHighlightCard from "./components/SummaryHighlightCard.vue";
import KbSelector from "./components/KbSelector.vue";
import DateRangePicker from "./components/DateRangePicker.vue";
import DeptSelector from "./components/DeptSelector.vue";
import iconTitle from '@/components/common/iconTitle.vue'
const { t } = useI18n();
const dateRange = ref<[string, string] | undefined>(undefined);

const { defaults, topUsers, topDocs, feedbackReasons, fallbackQuestions } = mockDashboard;
</script>

<template>
  <div class="dashboard-page page-shell page-shell--no-scroll">
    <!-- 顶部筛选区 -->
    <!-- <div class="dashboard-page__toolbar">
      <KbSelector />
      <DeptSelector :default-value="defaults.department" />
      <div class="dashboard-page__toolbar-spacer" />
      <button class="dashboard-page__link-btn">{{ t("dashboard.toolbar.exportSummary") }}</button>
      <button class="dashboard-page__link-btn">{{ t("dashboard.toolbar.exportReport") }}</button>
      <button class="dashboard-page__link-btn">{{ t("dashboard.toolbar.viewKbDetail") }}</button>
      <button class="dashboard-page__link-btn">{{ t("dashboard.toolbar.viewUserDetail") }}</button>
    </div> -->

    <!-- 6 个 KPI 卡片 -->
    <section class="dashboard-page__section">
      <div class="top-title-search">
        <iconTitle :title="t('exchangeRate.title')" />
        <KbSelector />
      </div>
      <KnowledgeBaseStats :items="mockDashboard.kbKpis" />
    </section>

    <!-- 6 个问答基本数据卡 -->
    <section class="dashboard-page__section">
      <div class="top-title-search">
        <iconTitle :title="t('exchangeRate.title')" />
        <div class="right-search">
          <DeptSelector :default-value="defaults.department" />
          <DateRangePicker v-model="dateRange" />
        </div>
      </div>
      <AnswerBasicData :items="mockDashboard.basicMetrics" />
    </section>

    <!-- 三图：领域分布 / 跨领域 / 有效回答环图 -->
    <section class="dashboard-page__charts">
      <div class="dashboard-page__chart-cell">
        <DomainPieChart :title="t('dashboard.chart.domainDist')" :data="mockDashboard.domainDistribution" />
      </div>
      <div class="dashboard-page__chart-cell">
        <DomainPieChart :title="t('dashboard.chart.crossDomain')" :data="mockDashboard.crossDomainDistribution" />
      </div>
      <div class="dashboard-page__chart-cell">
        <RingChart :data="mockDashboard.effectiveAnswerRing" />
      </div>
    </section>

    <!-- 双榜：Top10 用户 + Top10 文档 -->
    <section class="dashboard-page__rankings">
      <div class="dashboard-page__ranking-cell">
        <TopUsersRanking :items="topUsers" />
      </div>
      <div class="dashboard-page__ranking-cell">
        <TopDocsRanking :items="topDocs" />
      </div>
    </section>

    <!-- 产品反馈 + 兜底话术 -->
    <section class="dashboard-page__row">
      <div class="dashboard-page__row-cell">
        <FeedbackBarChartH :items="feedbackReasons" />
      </div>
      <div class="dashboard-page__row-cell">
        <FallbackQuestionsList :items="fallbackQuestions" />
      </div>
    </section>

    <!-- 17 天双柱状图 -->
    <section class="dashboard-page__section">
      <AnswerDetailData :title="t('dashboard.chart.answerTrend')" :left-legend="t('dashboard.chart.qas')"
        :right-legend="t('dashboard.chart.positiveRate')" :left-series="mockDashboard.monthlyTrend"
        :right-series="mockDashboard.startAnswerTrend" />
    </section>

    <!-- 总结大卡 -->
    <section class="dashboard-page__section">
      <SummaryHighlightCard />
    </section>
  </div>
</template>

<style scoped lang="less">
.dashboard-page {
  padding: 16px;
  background: #f5f6f9;
  min-height: 100%;
  overflow: auto;

  section {
    padding: 16px;
    background: white;
    border-radius: 8px;
    width: 100%;


  }

  .top-title-search {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 12px;

    .right-search {
      display: flex;
      align-items: center;
      justify-content: flex-end;
    }
  }

  &__toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
    flex-wrap: wrap;
  }

  &__toolbar-spacer {
    flex: 1;
  }

  &__link-btn {
    background: transparent;
    border: none;
    color: #0b41cd;
    font-size: 13px;
    font-family: "Noto Sans SC", sans-serif;
    cursor: pointer;
    padding: 6px 8px;

    &:hover {
      text-decoration: underline;
    }
  }

  &__section {
    margin-bottom: 16px;
  }

  &__charts {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 16px;
  }

  &__chart-cell {
    background: #ffffff;
    border-radius: 4px;
    padding: 16px;
    box-sizing: border-box;
    min-height: 240px;
  }

  &__rankings {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 16px;
  }

  &__ranking-cell {
    background: #ffffff;
    border-radius: 4px;
    padding: 16px;
    box-sizing: border-box;
  }

  &__row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 16px;
  }

  &__row-cell {
    background: #ffffff;
    border-radius: 4px;
    padding: 16px;
    box-sizing: border-box;
    min-height: 280px;
  }
}

@media (max-width: 1199px) {
  .dashboard-page {
    padding: 16px 16px 20px;
  }

  .dashboard-page__charts {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 767px) {
  .dashboard-page {
    padding: 12px 12px 16px;
  }

  .dashboard-page__charts,
  .dashboard-page__rankings,
  .dashboard-page__row {
    grid-template-columns: 1fr;
  }

  .dashboard-page__section {
    margin-bottom: 12px;
  }

  .dashboard-page__chart-cell,
  .dashboard-page__ranking-cell,
  .dashboard-page__row-cell {
    padding: 12px;
    min-height: 200px;
  }
}
</style>
