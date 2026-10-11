import { describe, expect, it } from 'vitest'

import {
  KEY_REDIRECT_RULES,
  WILDCARD_ONLY_RULES,
  firstModelMappingIssue,
  mappingRowsToRecord,
  overwriteSelfMappings,
  recordToMappingRows,
  validateModelMappingRows,
} from '../modelMappingRules'

describe('validateModelMappingRows', () => {
  it('未开启任何检查时不报告问题', () => {
    expect(validateModelMappingRows([{ from: '', to: 'a*b' }])).toEqual([{}])
  })

  it('按必填、长度、通配符、重复的顺序检查来源', () => {
    const tooLong = 'a'.repeat(101)
    const issues = validateModelMappingRows(
      [
        { from: '  ', to: 'x' },
        { from: tooLong, to: 'x' },
        { from: 'a*b', to: 'x' },
        { from: 'dup', to: 'x' },
        { from: ' dup ', to: 'y' },
      ],
      KEY_REDIRECT_RULES,
    )

    expect(issues.map((issue) => issue.from)).toEqual([
      'sourceRequired',
      'nameTooLong',
      'sourceWildcardInvalid',
      'duplicateSource',
      'duplicateSource',
    ])
  })

  it('按必填、长度、通配符、自映射的顺序检查目标', () => {
    const issues = validateModelMappingRows(
      [
        { from: 'a', to: '' },
        { from: 'b', to: 'x'.repeat(101) },
        { from: 'c', to: 'gpt-*' },
        { from: 'd', to: ' d ' },
      ],
      KEY_REDIRECT_RULES,
    )

    expect(issues.map((issue) => issue.to)).toEqual([
      'targetRequired',
      'nameTooLong',
      'targetWildcardInvalid',
      'selfMapping',
    ])
  })

  it('长度按码点计算', () => {
    const emoji = '😀'.repeat(100)
    expect(validateModelMappingRows([{ from: emoji, to: 'x' }], KEY_REDIRECT_RULES)).toEqual([{}])
  })

  it('允许单独的末尾通配符', () => {
    expect(validateModelMappingRows([{ from: '*', to: 'x' }], KEY_REDIRECT_RULES)).toEqual([{}])
    expect(validateModelMappingRows([{ from: 'gpt-**', to: 'x' }], KEY_REDIRECT_RULES)[0].from)
      .toBe('sourceWildcardInvalid')
  })

  it('仅通配符规则可以同时报告来源和目标', () => {
    expect(validateModelMappingRows([{ from: 'a*b', to: 'c*' }], WILDCARD_ONLY_RULES)).toEqual([
      { from: 'sourceWildcardInvalid', to: 'targetWildcardInvalid' },
    ])
    expect(validateModelMappingRows([{ from: '', to: '' }], WILDCARD_ONLY_RULES)).toEqual([{}])
  })
})

describe('firstModelMappingIssue', () => {
  it('按行序返回第一个问题并优先报告来源', () => {
    expect(firstModelMappingIssue([{}, { to: 'selfMapping', from: 'duplicateSource' }]))
      .toBe('duplicateSource')
    expect(firstModelMappingIssue([{}, {}])).toBeNull()
  })
})

describe('映射对象转换', () => {
  it('提交时去掉首尾空白', () => {
    expect(mappingRowsToRecord([{ from: ' a ', to: ' b ' }])).toEqual({ a: 'b' })
  })

  it('展开时保持键顺序并兼容空值', () => {
    expect(recordToMappingRows({ b: '1', a: '2' })).toEqual([
      { from: 'b', to: '1' },
      { from: 'a', to: '2' },
    ])
    expect(recordToMappingRows(null)).toEqual([])
  })
})

describe('overwriteSelfMappings', () => {
  it('用上游型号覆盖自映射', () => {
    expect(overwriteSelfMappings([{ from: 'old', to: 'old' }], ['a', 'b'])).toEqual([
      { from: 'a', to: 'a' },
      { from: 'b', to: 'b' },
    ])
  })

  it('保留 from≠to 的自定义改写，且不重复登记同来源的上游型号', () => {
    expect(
      overwriteSelfMappings(
        [
          { from: 'claude-opus-5', to: 'claude-opus-5-20260101' },
          { from: 'legacy', to: 'legacy' },
        ],
        ['legacy', 'claude-opus-5'],
      ),
    ).toEqual([
      { from: 'legacy', to: 'legacy' },
      { from: 'claude-opus-5', to: 'claude-opus-5-20260101' },
    ])
  })

  it('丢弃空来源与上游不再返回的自映射', () => {
    expect(
      overwriteSelfMappings(
        [
          { from: '', to: 'x' },
          { from: 'gone', to: 'gone' },
        ],
        ['keep'],
      ),
    ).toEqual([{ from: 'keep', to: 'keep' }])
  })
})
