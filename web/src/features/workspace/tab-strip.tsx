import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { X } from 'lucide-react'

export interface StripTab {
  key: string
  title: string
  color: string
  production: boolean
  connected: boolean
  visible: boolean
  focused: boolean
}

function SortableTab({
  tab,
  onSelect,
  onClose,
}: {
  tab: StripTab
  onSelect: () => void
  onClose: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: tab.key,
  })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      className={`session-tab host-${tab.color} ${tab.focused ? 'selected' : ''} ${tab.visible ? 'visible' : ''} ${isDragging ? 'dragging' : ''} ${tab.production ? 'production' : ''}`}
    >
      <button
        className="tab-select"
        {...attributes}
        {...listeners}
        aria-pressed={tab.focused}
        aria-roledescription="可拖动排序的标签"
        title={tab.connected ? tab.title : `${tab.title}（未连接）`}
        onClick={onSelect}
      >
        <span className={`tab-color ${tab.connected ? '' : 'offline'}`} />
        <span>{tab.title}</span>
      </button>
      <button className="tab-close" aria-label={`关闭标签 ${tab.title}`} onClick={onClose}>
        <X size={13} />
      </button>
    </div>
  )
}

/** Tabs can be reordered by mouse, touch or keyboard (space, arrows, space). */
export function TabStrip({
  tabs,
  onSelect,
  onClose,
  onReorder,
}: {
  tabs: StripTab[]
  onSelect: (key: string) => void
  onClose: (key: string) => void
  onReorder: (from: string, to: string) => void
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  function finish(event: DragEndEvent) {
    if (event.over && event.active.id !== event.over.id)
      onReorder(String(event.active.id), String(event.over.id))
  }
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={finish}>
      <SortableContext items={tabs.map((tab) => tab.key)} strategy={horizontalListSortingStrategy}>
        {tabs.map((tab) => (
          <SortableTab
            key={tab.key}
            tab={tab}
            onSelect={() => onSelect(tab.key)}
            onClose={() => onClose(tab.key)}
          />
        ))}
      </SortableContext>
    </DndContext>
  )
}
