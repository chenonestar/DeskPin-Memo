const P: Record<string, string> = {
  pin: 'M15 4l5 5-3 1-3.5 3.5.5 4-1.5 1.5-3.5-3.5L5 20.5M13 6l5 5',
  top: 'M12 19V6M6 11l6-6 6 6M5 21h14',
  window: 'M4 5h16v14H4zM4 9h16',
  menu: 'M5 12h.01M12 12h.01M19 12h.01',
  bell: 'M6 16V11a6 6 0 1112 0v5l1.5 2h-15L6 16zM10 21h4',
  repeat: 'M4 12V9a3 3 0 013-3h11l-3-3M20 12v3a3 3 0 01-3 3H6l3 3',
  search: 'M11 18a7 7 0 100-14 7 7 0 000 14zM21 21l-5-5',
  lock: 'M6 11h12v9H6zM8 11V8a4 4 0 118 0v3',
  check: 'M5 12.5l4.5 4.5L19 7.5',
  plus: 'M12 5v14M5 12h14',
  trash: 'M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13',
  chevron: 'M7 10l5 5 5-5',
  chevronRight: 'M10 7l5 5-5 5',
  x: 'M6 6l12 12M18 6L6 18',
}

export function Icon({ name, className = '' }: { name: keyof typeof P; className?: string }) {
  return (
    <svg className={`icon ${className}`} viewBox="0 0 24 24" aria-hidden="true">
      <path d={P[name]} />
    </svg>
  )
}
