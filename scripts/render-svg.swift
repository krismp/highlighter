// render-svg.swift SVG OUT SIZE: rasterize an SVG with AppKit.
// SIZE is N (square) or WxH. OUT ending in .bmp is written as a 24-bit BMP
// on white (the format the Windows installer's images must use); anything
// else is a PNG with transparency.
import AppKit

let a = CommandLine.arguments
guard let img = NSImage(contentsOfFile: a[1]) else { fatalError("cannot read \(a[1])") }
let dims = a[3].split(separator: "x").compactMap { Int($0) }
let (width, height) = dims.count == 2 ? (dims[0], dims[1]) : (dims[0], dims[0])
let bmp = a[2].lowercased().hasSuffix(".bmp")

let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8,
  samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: width * 4, bitsPerPixel: 32)!
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
NSGraphicsContext.current?.imageInterpolation = .high
let rect = NSRect(x: 0, y: 0, width: width, height: height)
if bmp {
  NSColor.white.setFill()
  rect.fill()
}
img.draw(in: rect)
NSGraphicsContext.restoreGraphicsState()

if !bmp {
  try! rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: a[2]))
  exit(0)
}

// 24-bit BMP: 54-byte header, then bottom-up rows of BGR padded to 4 bytes.
// (Core Graphics cannot draw into a 24-bit bitmap, so it is converted here.)
let rowSize = (width * 3 + 3) & ~3
var out = Data()
func le32(_ v: Int) { var x = UInt32(v).littleEndian; out.append(Data(bytes: &x, count: 4)) }
func le16(_ v: Int) { var x = UInt16(v).littleEndian; out.append(Data(bytes: &x, count: 2)) }
out.append(contentsOf: [0x42, 0x4D]); le32(54 + rowSize * height); le32(0); le32(54)
le32(40); le32(width); le32(height); le16(1); le16(24); le32(0); le32(rowSize * height); le32(2835); le32(2835); le32(0); le32(0)
let px = rep.bitmapData!
for y in stride(from: height - 1, through: 0, by: -1) {
  var row = [UInt8](repeating: 0, count: rowSize)
  for x in 0..<width {
    let p = px + (y * width + x) * 4
    row[x * 3] = p[2]; row[x * 3 + 1] = p[1]; row[x * 3 + 2] = p[0]
  }
  out.append(contentsOf: row)
}
try! out.write(to: URL(fileURLWithPath: a[2]))
