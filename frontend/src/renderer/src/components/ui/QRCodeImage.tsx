import * as QRCode from 'qrcode'

// Renders nothing when the value exceeds QR capacity (about 2.3 KB).
export function QRCodeImage({ value, label, className = 'size-full' }: { value: string; label: string; className?: string }) {
  let modules: QRCode.BitMatrix
  try {
    modules = QRCode.create(value, { errorCorrectionLevel: 'M' }).modules
  } catch {
    return null
  }
  const { size, data } = modules
  let path = ''
  data.forEach((dark, index) => {
    if (dark) {
      path += `M${index % size} ${Math.floor(index / size)}h1v1h-1z`
    }
  })
  return <svg role="img" aria-label={label} viewBox={`-1 -1 ${size + 2} ${size + 2}`} shapeRendering="crispEdges" className={className}>
    <path d={path} fill="#111" />
  </svg>
}
