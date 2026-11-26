import type { ReactNode } from 'react'

export function SettingsHeading({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <header className="settings-heading">
      <h3>{title}</h3>
      {children && <p>{children}</p>}
    </header>
  )
}
