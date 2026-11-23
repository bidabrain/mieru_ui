// Draws the app icon into an .iconset directory: swift make-icon.swift <out.iconset>
import AppKit

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "AppIcon.iconset"
try? FileManager.default.createDirectory(atPath: out, withIntermediateDirectories: true)

func render(_ px: Int) -> Data {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                               colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    let s = CGFloat(px)
    // macOS icon grid: 824/1024 body with rounded corners
    let inset = s * 100 / 1024
    let body = NSRect(x: inset, y: inset, width: s - 2 * inset, height: s - 2 * inset)
    let path = NSBezierPath(roundedRect: body, xRadius: body.width * 0.225, yRadius: body.width * 0.225)
    let grad = NSGradient(colors: [
        NSColor(calibratedRed: 0.36, green: 0.42, blue: 0.98, alpha: 1),
        NSColor(calibratedRed: 0.55, green: 0.30, blue: 0.93, alpha: 1),
    ])!
    grad.draw(in: path, angle: -60)

    // a stylised eye (見える) made of two arcs and a pupil
    let c = NSPoint(x: s / 2, y: s / 2)
    let w = body.width * 0.62, h = body.width * 0.34
    let eye = NSBezierPath()
    eye.move(to: NSPoint(x: c.x - w / 2, y: c.y))
    eye.curve(to: NSPoint(x: c.x + w / 2, y: c.y), controlPoint1: NSPoint(x: c.x - w / 4, y: c.y + h), controlPoint2: NSPoint(x: c.x + w / 4, y: c.y + h))
    eye.curve(to: NSPoint(x: c.x - w / 2, y: c.y), controlPoint1: NSPoint(x: c.x + w / 4, y: c.y - h), controlPoint2: NSPoint(x: c.x - w / 4, y: c.y - h))
    NSColor.white.setFill()
    eye.fill()
    let r = h * 0.55
    let pupil = NSBezierPath(ovalIn: NSRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r))
    NSColor(calibratedRed: 0.40, green: 0.36, blue: 0.96, alpha: 1).setFill()
    pupil.fill()
    let r2 = r * 0.38
    NSColor.white.setFill()
    NSBezierPath(ovalIn: NSRect(x: c.x - r2 + r * 0.3, y: c.y - r2 + r * 0.3, width: 2 * r2, height: 2 * r2)).fill()

    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

for size in [16, 32, 128, 256, 512] {
    try! render(size).write(to: URL(fileURLWithPath: "\(out)/icon_\(size)x\(size).png"))
    try! render(size * 2).write(to: URL(fileURLWithPath: "\(out)/icon_\(size)x\(size)@2x.png"))
}
print("wrote \(out)")
