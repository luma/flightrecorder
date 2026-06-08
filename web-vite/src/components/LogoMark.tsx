interface LogoMarkProps {
  className?: string
  size?: number
}

export default function LogoMark({ className = '', size = 24 }: LogoMarkProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      className={className}
      aria-label="flightrecorder logo"
    >
      <g strokeLinecap="round" strokeLinejoin="round">
        {/* map pin */}
        <path
          d="M32 7 C 21 7 12 16 12 27 C 12 39 24 49 32 58 C 40 49 52 39 52 27 C 52 16 43 7 32 7 Z"
          stroke="#ffffff"
          strokeWidth={3.2}
        />
        {/* telemetry / ECG waveform */}
        <path
          d="M14 28 H22 L24.5 24 L26.5 33 L28.5 13 L30.5 40 L32.5 28 H39.5"
          stroke="#ff6a00"
          strokeWidth={2.8}
        />
      </g>
      {/* record dot */}
      <circle cx="42.5" cy="28" r="2.3" fill="#14b8a6" />
    </svg>
  )
}
