import assert from 'node:assert/strict'
import test from 'node:test'

import {
  classifyDateBucket,
  groupSessions,
} from './sessionGrouping.ts'

test('groupSessions date mode keeps pinned sessions in their own bucket', () => {
  const now = new Date().toISOString()
  const groups = groupSessions(
    'date',
    [
      { id: 'p', is_pinned: true, updated_at: now },
      { id: 't', updated_at: now },
    ],
    {
      pinnedLabel: 'Pinned',
      bucketLabels: {
        pinned: 'Pinned',
        today: 'Today',
        yesterday: 'Yesterday',
        last7Days: '7d',
        last30Days: '30d',
        lastYear: 'Year',
        earlier: 'Earlier',
      },
      categorizeDate: () => 'today',
    },
  )
  assert.deepEqual(
    groups.map((g) => g.key),
    ['pinned', 'today'],
  )
})

test('classifyDateBucket buckets recent sessions as today', () => {
  assert.equal(classifyDateBucket(new Date().toISOString()), 'today')
})
