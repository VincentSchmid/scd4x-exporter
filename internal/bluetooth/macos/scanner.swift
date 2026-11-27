import CoreBluetooth
import Foundation

final class Scanner: NSObject, CBCentralManagerDelegate {
    func centralManagerDidUpdateState(_ central: CBCentralManager) {
        switch central.state {
        case .unknown:
            return
        case .poweredOn:
            central.scanForPeripherals(withServices: nil, options: [
                CBCentralManagerScanOptionAllowDuplicatesKey: true
            ])
            send("ready")
        case .unauthorized:
            fail("Bluetooth access denied. Allow Bluetooth for the app running this command in System Settings > Privacy & Security > Bluetooth.")
        case .poweredOff:
            fail("Bluetooth is turned off.")
        case .unsupported:
            fail("This Mac does not support Bluetooth LE scanning.")
        case .resetting:
            fail("Bluetooth is resetting.")
        @unknown default:
            fail("Bluetooth is unavailable.")
        }
    }

    func centralManager(_ central: CBCentralManager, didDiscover peripheral: CBPeripheral,
                        advertisementData: [String: Any], rssi RSSI: NSNumber) {
        guard let data = advertisementData[CBAdvertisementDataManufacturerDataKey] as? Data else { return }
        send(data.map { String(format: "%02x", $0) }.joined())
    }

    private func send(_ line: String) {
        FileHandle.standardOutput.write(Data((line + "\n").utf8))
    }

    private func fail(_ message: String) -> Never {
        FileHandle.standardError.write(Data((message + "\n").utf8))
        exit(1)
    }
}

let scanner = Scanner()
let central = CBCentralManager(delegate: scanner, queue: nil)
withExtendedLifetime((scanner, central)) {
    RunLoop.main.run()
}
