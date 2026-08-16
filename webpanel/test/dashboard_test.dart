import 'package:clearlink_panel/main.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('dashboard renders topology sections and active nodes', (tester) async {
    await tester.pumpWidget(
      const ClearLinkAdminApp(
        initialTopology: TopologyResponse(
          listen: [
            PublicNode(peerId: 10, name: 'North Tower', active: true, rssi: -18),
            PublicNode(peerId: 11, name: 'South Tower', active: false, rssi: -42),
          ],
          server: PublicNode(peerId: 0, name: 'Server', active: true, rssi: null),
          broadcast: [
            PublicNode(peerId: 20, name: 'Relay A', active: true, rssi: null),
            PublicNode(peerId: 21, name: 'Relay B', active: true, rssi: null),
          ],
        ),
      ),
    );

    expect(find.text('Dashboard'), findsOneWidget);
    expect(find.text('North Tower'), findsOneWidget);
    expect(find.text('Server'), findsOneWidget);
    expect(find.text('Relay A'), findsOneWidget);
    expect(find.text('RSSI -18 dB'), findsOneWidget);
    expect(find.byType(TopologyDiagram), findsOneWidget);
  });
}
