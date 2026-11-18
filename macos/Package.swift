// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Taasant",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(name: "Taasant", resources: [
            .copy("Resources/ant.png"),
            .copy("Resources/ant-menu.png"),
            .copy("Resources/ant-menu-on.png"),
        ])
    ]
)
