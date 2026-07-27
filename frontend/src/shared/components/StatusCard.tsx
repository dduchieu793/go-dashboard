type StatusCardProps = {
  label: string
  available: boolean
  detail?: string
}

export function StatusCard({ label, available, detail }: StatusCardProps) {
  return (
    <article className="status-card">
      <div>
        <p className="status-label">{label}</p>
        {detail && <p className="status-detail">{detail}</p>}
      </div>
      <span className={`status ${available ? 'available' : 'unavailable'}`}>
        <span className="status-dot" aria-hidden="true" />
        {available ? 'Available' : 'Unavailable'}
      </span>
    </article>
  )
}
