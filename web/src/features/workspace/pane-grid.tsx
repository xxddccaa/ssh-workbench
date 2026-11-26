import type { ReactNode } from 'react'
import { Group, Panel, Separator, type Layout } from 'react-resizable-panels'
import type { Split } from './layout'

function percent(value: number | undefined, fallback: number) {
  return value !== undefined && value >= 5 && value <= 95 ? value : fallback
}

/**
 * Lays out up to four panes. Sizes are percentages persisted with the
 * workspace; panes never shrink below 15% so no terminal is resized to 0×0.
 */
export function PaneGrid({
  split,
  sizes,
  onSizes,
  render,
}: {
  split: Split
  sizes: number[]
  onSizes: (sizes: number[]) => void
  render: (slot: number) => ReactNode
}) {
  if (split === 'single') return <div className="pane-single">{render(0)}</div>
  const pair = (
    id: string,
    orientation: 'horizontal' | 'vertical',
    first: ReactNode,
    second: ReactNode,
    stored: [number | undefined, number | undefined],
    store: (layout: [number, number]) => void,
  ) => {
    const defaultLayout: Layout = {
      [`${id}-a`]: percent(stored[0], 50),
      [`${id}-b`]: percent(stored[1], 50),
    }
    return (
      <Group
        id={id}
        orientation={orientation}
        className="pane-group"
        defaultLayout={defaultLayout}
        onLayoutChanged={(layout) => store([layout[`${id}-a`] ?? 50, layout[`${id}-b`] ?? 50])}
      >
        <Panel id={`${id}-a`} minSize="15" className="pane-cell">
          {first}
        </Panel>
        <Separator className={`pane-separator ${orientation}`} />
        <Panel id={`${id}-b`} minSize="15" className="pane-cell">
          {second}
        </Panel>
      </Group>
    )
  }
  if (split === 'columns' || split === 'rows')
    return pair(
      split,
      split === 'columns' ? 'horizontal' : 'vertical',
      render(0),
      render(1),
      [sizes[0], sizes[1]],
      (layout) => onSizes(layout),
    )
  const update = (offset: number, layout: [number, number]) => {
    const next = [0, 1, 2, 3, 4, 5].map((index) => percent(sizes[index], 50))
    next[offset] = layout[0]
    next[offset + 1] = layout[1]
    onSizes(next)
  }
  return pair(
    'grid-rows',
    'vertical',
    pair('grid-top', 'horizontal', render(0), render(1), [sizes[2], sizes[3]], (layout) =>
      update(2, layout),
    ),
    pair('grid-bottom', 'horizontal', render(2), render(3), [sizes[4], sizes[5]], (layout) =>
      update(4, layout),
    ),
    [sizes[0], sizes[1]],
    (layout) => update(0, layout),
  )
}
