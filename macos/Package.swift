// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Mieru",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "Mieru",
            path: "Sources/Mieru"
        ),
    ]
)
