import 'package:clearlink_panel/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('dashboard renders topology sections and active nodes', (
    tester,
  ) async {
    await tester.pumpWidget(
      const ClearLinkAdminApp(
        initialTopology: TopologyResponse(
          listen: [
            PublicNode(
              peerId: 10,
              name: 'North Tower',
              active: true,
              rssi: -18,
              snr: 12,
              lastKeyUpAgo: '12s',
            ),
            PublicNode(
              peerId: 11,
              name: 'South Tower',
              active: false,
              rssi: -42,
              snr: null,
            ),
          ],
          interfaceNodes: [
            PublicNode(
              peerId: 15,
              name: 'Dispatch Gateway',
              active: true,
              rssi: null,
              snr: null,
            ),
          ],
          server: PublicNode(
            peerId: 0,
            name: 'Server',
            active: true,
            rssi: null,
            snr: null,
          ),
          broadcast: [
            PublicNode(
              peerId: 20,
              name: 'Relay A',
              active: true,
              rssi: null,
              snr: null,
            ),
            PublicNode(
              peerId: 21,
              name: 'Relay B',
              active: true,
              rssi: null,
              snr: null,
            ),
          ],
        ),
      ),
    );

    await tester.pump();

    expect(find.text('Dashboard'), findsOneWidget);
    expect(find.text('North Tower'), findsOneWidget);
    expect(find.text('Dispatch Gateway'), findsOneWidget);
    expect(find.text('Server'), findsOneWidget);
    expect(find.text('Relay A'), findsOneWidget);
    expect(find.text('RSSI -18 dB | SNR 12.0 dB'), findsOneWidget);
    expect(find.text('Last keyup: 12s ago'), findsOneWidget);
    expect(find.text('Interface'), findsOneWidget);
    expect(find.byType(TopologyDiagram), findsOneWidget);
  });

  testWidgets('admin configuration stages selected node settings', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: AdminConfigTab(
            api: const AdminApi(baseUrl: 'http://localhost:8080'),
            nodes: const [
              NodeConfigViewModel(
                peerId: 7,
                name: 'North Tower',
                nodeType: 'listen',
                remoteAddr: '10.0.0.7:4125',
                lastHeartbeat: '',
                lastHeartbeatAgo: '3s',
                active: true,
                rssi: -32,
                snr: 8.5,
                config: [
                  NodeConfigEntryViewModel(
                    key: 'AudioRate',
                    type: 'listen',
                    value: '48000',
                    valueType: 'int',
                  ),
                  NodeConfigEntryViewModel(
                    key: 'RouteToBroadcast',
                    type: 'listen',
                    value: 'false',
                    valueType: 'bool',
                  ),
                ],
              ),
            ],
            onRefresh: _noOpRefresh,
          ),
        ),
      ),
    );

    expect(find.text('Connected nodes'), findsOneWidget);
    expect(find.text('North Tower'), findsWidgets);
    expect(find.text('Audio Rate'), findsWidgets);
    expect(find.text('Route To Broadcast'), findsWidgets);

    final routeToBroadcastSwitch = find.byType(Switch);
    await tester.ensureVisible(routeToBroadcastSwitch);
    await tester.tap(routeToBroadcastSwitch);
    await tester.pump();

    expect(find.text('1 pending'), findsOneWidget);
    expect(find.text('Save changes'), findsOneWidget);
  });
}

Future<void> _noOpRefresh() async {}
