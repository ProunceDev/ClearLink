import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

void main() {
  runApp(const ClearLinkAdminApp());
}

class ClearLinkAdminApp extends StatelessWidget {
  final TopologyResponse? initialTopology;

  const ClearLinkAdminApp({super.key, this.initialTopology});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'ClearLink Admin',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        brightness: Brightness.dark,
        scaffoldBackgroundColor: const Color(0xFF0B1220),
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF2DD4BF),
          brightness: Brightness.dark,
        ),
      ),
      home: DashboardPage(initialTopology: initialTopology),
    );
  }
}

class DashboardPage extends StatefulWidget {
  final TopologyResponse? initialTopology;

  const DashboardPage({super.key, this.initialTopology});

  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> {
  late Future<_DashboardData> _dashboardFuture;

  @override
  void initState() {
    super.initState();
    _dashboardFuture = _loadDashboard();
  }

  Future<_DashboardData> _loadDashboard() async {
    if (widget.initialTopology != null) {
      return _DashboardData(topology: widget.initialTopology!, connections: const []);
    }

    final baseUrl = _getBaseUrl();
    final topologyRes = await http.get(Uri.parse('$baseUrl/api/public/topology'));
    if (topologyRes.statusCode != 200) {
      throw Exception('Failed to load topology: ${topologyRes.statusCode}');
    }

    final topology = TopologyResponse.fromJson(jsonDecode(topologyRes.body));

    final connectionsRes = await http.get(Uri.parse('$baseUrl/api/public/connections'));
    List<TopologyConnection> connections = const [];
    if (connectionsRes.statusCode == 200) {
      final decoded = jsonDecode(connectionsRes.body);
      if (decoded is Map<String, dynamic> && decoded['connections'] is List) {
        connections = (decoded['connections'] as List)
            .map((entry) => TopologyConnection.fromJson(entry as Map<String, dynamic>))
            .toList();
      }
    }

    return _DashboardData(topology: topology, connections: connections);
  }

  String _getBaseUrl() {
    final uri = Uri.base;
    final hasOrigin = uri.origin.isNotEmpty && uri.origin != 'null';
    return hasOrigin ? uri.origin : 'http://localhost:8080';
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: DecoratedBox(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Color(0xFF0A1120), Color(0xFF101A2E)],
          ),
        ),
        child: SafeArea(
          child: FutureBuilder<_DashboardData>(
            future: _dashboardFuture,
            builder: (context, snapshot) {
              if (snapshot.connectionState == ConnectionState.waiting) {
                return const Center(child: CircularProgressIndicator());
              }

              if (snapshot.hasError) {
                return Center(
                  child: Card(
                    color: const Color(0xFF1C2435),
                    child: Padding(
                      padding: const EdgeInsets.all(24),
                      child: Text('Unable to load dashboard\n${snapshot.error}', textAlign: TextAlign.center),
                    ),
                  ),
                );
              }

              final data = snapshot.data ??
                  _DashboardData(
                    topology: TopologyResponse.empty(),
                    connections: const [],
                  );
              return Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const SizedBox(height: 12),
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            'Dashboard',
                            style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                              letterSpacing: 0.5,
                            ),
                          ),
                        ),
                        _MetricBadge(label: 'Listen', value: '${data.topology.listen.length}'),
                        const SizedBox(width: 12),
                        _MetricBadge(label: 'Broadcast', value: '${data.topology.broadcast.length}'),
                        const SizedBox(width: 12),
                        _MetricBadge(label: 'Active', value: '${_countActive(data.topology)}'),
                        const SizedBox(width: 12),
                        IconButton(
                          onPressed: () => _showAdminPanel(context),
                          tooltip: 'Admin panel',
                          style: IconButton.styleFrom(
                            backgroundColor: const Color(0xFF111B2B),
                            foregroundColor: Colors.white,
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(14),
                            ),
                            padding: const EdgeInsets.all(12),
                          ),
                          icon: const Icon(Icons.settings_outlined),
                        ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    Expanded(
                      child: TopologyDiagram(
                        topology: data.topology,
                        connections: data.connections,
                      ),
                    ),
                  ],
                ),
              );
            },
          ),
        ),
      ),
    );
  }

  void _showAdminPanel(BuildContext context) {
    final baseUrl = _getBaseUrl();

    final usernameController = TextEditingController();
    final passwordController = TextEditingController();

    showDialog(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          backgroundColor: const Color(0xFF101A2E),
          title: const Text('Admin login', style: TextStyle(color: Colors.white)),
          content: SizedBox(
            width: 360,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: usernameController,
                  style: const TextStyle(color: Colors.white),
                  decoration: const InputDecoration(
                    labelText: 'Username',
                    labelStyle: TextStyle(color: Color(0xFFA9B7C9)),
                  ),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: passwordController,
                  obscureText: true,
                  style: const TextStyle(color: Colors.white),
                  decoration: const InputDecoration(
                    labelText: 'Password',
                    labelStyle: TextStyle(color: Color(0xFFA9B7C9)),
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () async {
                final username = usernameController.text.trim();
                final password = passwordController.text;
                if (username.isEmpty || password.isEmpty) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Username and password are required')),
                  );
                  return;
                }

                try {
                  final response = await http.post(
                    Uri.parse('$baseUrl/api/admin/login'),
                    headers: {'Content-Type': 'application/json'},
                    body: jsonEncode({
                      'username': username,
                      'password': password,
                    }),
                  );

                  if (!mounted) return;
                  if (response.statusCode != 200) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(content: Text('Invalid admin credentials')),
                    );
                    return;
                  }

                  Navigator.of(dialogContext).pop();

                  final nodesResponse = await http.get(
                    Uri.parse('$baseUrl/api/admin/nodes'),
                    headers: {'Authorization': _basicAuthHeader(username, password)},
                  );

                  if (!mounted) return;
                  if (nodesResponse.statusCode != 200) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(content: Text('Unable to load connected nodes')),
                    );
                    return;
                  }

                  final decoded = jsonDecode(nodesResponse.body);
                  final nodes = (decoded as List)
                      .map((entry) => NodeConfigViewModel.fromJson(entry as Map<String, dynamic>))
                      .toList();

                  showModalBottomSheet(
                    context: context,
                    isScrollControlled: true,
                    backgroundColor: Colors.transparent,
                    builder: (_) => AdminPanelSheet(
                      nodes: nodes,
                      username: username,
                      password: password,
                    ),
                  );
                } catch (_) {
                  if (!mounted) return;
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Unable to reach the admin API')),
                  );
                }
              },
              child: const Text('Login'),
            ),
          ],
        );
      },
    );
  }

  String _basicAuthHeader(String username, String password) {
    final encoded = base64Encode(utf8.encode('$username:$password'));
    return 'Basic $encoded';
  }
}

class _DashboardData {
  final TopologyResponse topology;
  final List<TopologyConnection> connections;

  const _DashboardData({required this.topology, required this.connections});
}

class _MetricBadge extends StatelessWidget {
  final String label;
  final String value;

  const _MetricBadge({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 10),
      decoration: BoxDecoration(
        color: const Color(0xFF111B2B),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(color: Colors.white60, fontSize: 11, letterSpacing: 0.8),
          ),
          const SizedBox(height: 4),
          Text(
            value,
            style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 20),
          ),
        ],
      ),
    );
  }
}

int _countActive(TopologyResponse topology) {
  final activeListen = topology.listen.where((node) => node.active).length;
  final activeBroadcast = topology.broadcast.where((node) => node.active).length;
  return activeListen + activeBroadcast + (topology.server.active ? 1 : 0);
}

class TopologyDiagram extends StatelessWidget {
  final TopologyResponse topology;
  final List<TopologyConnection> connections;

  const TopologyDiagram({super.key, required this.topology, required this.connections});

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        const boxWidth = 170.0;
        const boxHeight = 84.0;
        const rowGap = 18.0;
        const serverWidth = 210.0;
        const serverHeight = 96.0;

        final listenWidth = topology.listen.isEmpty
            ? boxWidth
            : topology.listen.length * boxWidth + (topology.listen.length - 1) * rowGap;
        final broadcastWidth = topology.broadcast.isEmpty
            ? boxWidth
            : topology.broadcast.length * boxWidth + (topology.broadcast.length - 1) * rowGap;

        final listenStart = (constraints.maxWidth - listenWidth) / 2;
        final broadcastStart = (constraints.maxWidth - broadcastWidth) / 2;
        final serverX = (constraints.maxWidth - serverWidth) / 2;

        final listenRowTop = 40.0;
        final serverRowTop = 180.0;
        final broadcastRowTop = 325.0;

        return SizedBox(
          width: constraints.maxWidth,
          height: 460,
          child: Stack(
            children: [
              Positioned.fill(
                child: CustomPaint(
                  painter: TopologyPainter(
                    listenNodes: topology.listen,
                    server: topology.server,
                    broadcastNodes: topology.broadcast,
                    listenRowTop: listenRowTop,
                    serverRowTop: serverRowTop,
                    broadcastRowTop: broadcastRowTop,
                    boxWidth: boxWidth,
                    boxHeight: boxHeight,
                    serverWidth: serverWidth,
                    serverHeight: serverHeight,
                    listenStart: listenStart,
                    broadcastStart: broadcastStart,
                    serverX: serverX,
                  ),
                ),
              ),
              Positioned(
                top: listenRowTop,
                left: listenStart,
                child: Wrap(
                  spacing: rowGap,
                  children: topology.listen.isEmpty
                      ? [
                          _NodeCard(
                            node: const PublicNode(
                              peerId: 0,
                              name: 'No listen nodes',
                              active: false,
                              rssi: null,
                            ),
                            compact: true,
                          ),
                        ]
                      : topology.listen
                          .map((node) => _NodeCard(node: node, width: boxWidth, height: boxHeight))
                          .toList(),
                ),
              ),
              Positioned(
                top: serverRowTop,
                left: serverX,
                child: _NodeCard(
                  node: topology.server,
                  width: serverWidth,
                  height: serverHeight,
                  isServer: true,
                ),
              ),
              Positioned(
                top: broadcastRowTop,
                left: broadcastStart,
                child: Wrap(
                  spacing: rowGap,
                  children: topology.broadcast.isEmpty
                      ? [
                          _NodeCard(
                            node: const PublicNode(
                              peerId: 0,
                              name: 'No broadcast nodes',
                              active: false,
                              rssi: null,
                            ),
                            compact: true,
                          ),
                        ]
                      : topology.broadcast
                          .map((node) => _NodeCard(node: node, width: boxWidth, height: boxHeight))
                          .toList(),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class TopologyPainter extends CustomPainter {
  final List<PublicNode> listenNodes;
  final PublicNode server;
  final List<PublicNode> broadcastNodes;
  final double listenRowTop;
  final double serverRowTop;
  final double broadcastRowTop;
  final double boxWidth;
  final double boxHeight;
  final double serverWidth;
  final double serverHeight;
  final double listenStart;
  final double broadcastStart;
  final double serverX;

  TopologyPainter({
    required this.listenNodes,
    required this.server,
    required this.broadcastNodes,
    required this.listenRowTop,
    required this.serverRowTop,
    required this.broadcastRowTop,
    required this.boxWidth,
    required this.boxHeight,
    required this.serverWidth,
    required this.serverHeight,
    required this.listenStart,
    required this.broadcastStart,
    required this.serverX,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final mutedPaint = Paint()
      ..color = const Color(0xFF68768B)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round;

    final activePaint = Paint()
      ..color = const Color(0xFFFF4D4D)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round
      ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 2);

    for (var i = 0; i < listenNodes.length; i++) {
      final node = listenNodes[i];
      final start = Offset(listenStart + (i * (boxWidth + 18)) + (boxWidth / 2), listenRowTop + boxHeight);
      final end = Offset(serverX + (serverWidth / 2), serverRowTop);
      final path = Path();
      path.moveTo(start.dx, start.dy);
      path.cubicTo(
        start.dx,
        start.dy + 46,
        end.dx,
        end.dy - 46,
        end.dx,
        end.dy,
      );
      canvas.drawPath(path, node.active ? activePaint : mutedPaint);
    }

    for (var i = 0; i < broadcastNodes.length; i++) {
      final node = broadcastNodes[i];
      final start = Offset(serverX + (serverWidth / 2), serverRowTop + serverHeight);
      final end = Offset(broadcastStart + (i * (boxWidth + 18)) + (boxWidth / 2), broadcastRowTop);
      final path = Path();
      path.moveTo(start.dx, start.dy);
      path.cubicTo(
        start.dx,
        start.dy + 44,
        end.dx,
        end.dy - 44,
        end.dx,
        end.dy,
      );
      canvas.drawPath(path, node.active ? activePaint : mutedPaint);
    }
  }

  @override
  bool shouldRepaint(covariant TopologyPainter oldDelegate) => false;
}

class _NodeCard extends StatelessWidget {
  final PublicNode node;
  final double width;
  final double height;
  final bool isServer;
  final bool compact;

  const _NodeCard({
    required this.node,
    this.width = 170,
    this.height = 84,
    this.isServer = false,
    this.compact = false,
  });

  @override
  Widget build(BuildContext context) {
    final active = node.active;
    final borderColor = active ? const Color(0xFFFF4D4D) : Colors.white.withValues(alpha: 0.12);

    return Container(
      width: width,
      height: height,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: isServer ? const Color(0xFF171F30) : const Color(0xFF101B2A),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: borderColor, width: active ? 1.4 : 1),
        boxShadow: [
          BoxShadow(
            color: (active ? const Color(0xFFFF4D4D) : const Color(0xFF3B4658)).withValues(alpha: 0.25),
            blurRadius: active ? 18 : 10,
            spreadRadius: active ? 0.5 : 0,
          ),
        ],
      ),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 10,
                height: 10,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: active ? const Color(0xFFFF4D4D) : const Color(0xFF69778B),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  node.name,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.w600,
                    fontSize: 15,
                  ),
                ),
              ),
            ],
          ),
          if (!isServer && node.rssi != null) ...[
            const SizedBox(height: 12),
            Text(
              'RSSI ${node.rssi!.toStringAsFixed(0)} dB',
              style: const TextStyle(
                color: Color(0xFFA9B7C9),
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ] else if (isServer) ...[
            const SizedBox(height: 8),
            const Text(
              'System core',
              style: TextStyle(
                color: Color(0xFF9DB6C7),
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class TopologyResponse {
  final List<PublicNode> listen;
  final PublicNode server;
  final List<PublicNode> broadcast;

  const TopologyResponse({
    required this.listen,
    required this.server,
    required this.broadcast,
  });

  factory TopologyResponse.empty() {
    return TopologyResponse(
      listen: const [],
      server: const PublicNode(peerId: 0, name: 'Server', active: false, rssi: null),
      broadcast: const [],
    );
  }

  factory TopologyResponse.fromJson(Map<String, dynamic> json) {
    final listenRaw = json['listen'] as List? ?? const [];
    final broadcastRaw = json['broadcast'] as List? ?? const [];
    final serverRaw = json['server'] as Map<String, dynamic>? ?? const {};

    return TopologyResponse(
      listen: listenRaw.map((entry) => PublicNode.fromJson(entry as Map<String, dynamic>)).toList(),
      server: PublicNode.fromJson(serverRaw),
      broadcast: broadcastRaw.map((entry) => PublicNode.fromJson(entry as Map<String, dynamic>)).toList(),
    );
  }
}

class PublicNode {
  final int peerId;
  final String name;
  final bool active;
  final double? rssi;

  const PublicNode({required this.peerId, required this.name, required this.active, required this.rssi});

  factory PublicNode.fromJson(Map<String, dynamic> json) {
    final peerIdValue = (json['peerId'] as num?)?.toInt() ?? 0;
    final rawRssi = json['rssi'];
    double? parsedRssi;
    if (rawRssi != null) {
      parsedRssi = (rawRssi as num).toDouble();
    }

    final requestedName = (json['name'] as String? ?? 'Peer $peerIdValue').trim();

    return PublicNode(
      peerId: peerIdValue,
      name: requestedName.isEmpty ? 'Peer $peerIdValue' : requestedName,
      active: json['active'] as bool? ?? false,
      rssi: parsedRssi,
    );
  }
}

class TopologyConnection {
  final String id;
  final String fromNodeId;
  final String toNodeId;
  final String color;
  final double width;

  const TopologyConnection({
    required this.id,
    required this.fromNodeId,
    required this.toNodeId,
    required this.color,
    required this.width,
  });

  factory TopologyConnection.fromJson(Map<String, dynamic> json) {
    return TopologyConnection(
      id: json['id'] as String? ?? '',
      fromNodeId: json['fromNodeId'] as String? ?? '',
      toNodeId: json['toNodeId'] as String? ?? '',
      color: json['color'] as String? ?? '#6B7280',
      width: (json['width'] as num?)?.toDouble() ?? 2,
    );
  }
}

class NodeConfigViewModel {
  final int peerId;
  final String name;
  final String nodeType;
  final String remoteAddr;
  final String lastHeartbeat;
  final String lastHeartbeatAgo;
  final bool active;
  final double? rssi;
  final List<NodeConfigEntryViewModel> config;

  const NodeConfigViewModel({
    required this.peerId,
    required this.name,
    required this.nodeType,
    required this.remoteAddr,
    required this.lastHeartbeat,
    required this.lastHeartbeatAgo,
    required this.active,
    required this.rssi,
    required this.config,
  });

  factory NodeConfigViewModel.fromJson(Map<String, dynamic> json) {
    final rawConfig = json['config'] as List? ?? const [];
    return NodeConfigViewModel(
      peerId: (json['peerId'] as num?)?.toInt() ?? 0,
      name: json['name'] as String? ?? 'Peer ${json['peerId'] ?? 0}',
      nodeType: json['nodeType'] as String? ?? 'unknown',
      remoteAddr: json['remoteAddr'] as String? ?? 'unknown',
      lastHeartbeat: json['lastHeartbeat'] as String? ?? '',
      lastHeartbeatAgo: json['lastHeartbeatAgo'] as String? ?? '',
      active: json['active'] as bool? ?? false,
      rssi: (json['rssi'] is num) ? (json['rssi'] as num).toDouble() : null,
      config: rawConfig
          .map((entry) => NodeConfigEntryViewModel.fromJson(entry as Map<String, dynamic>))
          .toList(),
    );
  }
}

class NodeConfigEntryViewModel {
  final String key;
  final String type;
  final String value;

  const NodeConfigEntryViewModel({
    required this.key,
    required this.type,
    required this.value,
  });

  factory NodeConfigEntryViewModel.fromJson(Map<String, dynamic> json) {
    return NodeConfigEntryViewModel(
      key: json['key'] as String? ?? '',
      type: json['type'] as String? ?? '',
      value: json['value'] as String? ?? '',
    );
  }
}

class AdminPanelSheet extends StatefulWidget {
  final List<NodeConfigViewModel> nodes;
  final String username;
  final String password;

  const AdminPanelSheet({
    super.key,
    required this.nodes,
    required this.username,
    required this.password,
  });

  @override
  State<AdminPanelSheet> createState() => _AdminPanelSheetState();
}

class _AdminPanelSheetState extends State<AdminPanelSheet> {
  final Map<String, TextEditingController> _controllers = {};
  final Map<String, String> _saveQueue = {};

  @override
  void dispose() {
    for (final controller in _controllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final nodes = widget.nodes;

    return Container(
      height: MediaQuery.of(context).size.height * 0.85,
      decoration: const BoxDecoration(
        color: Color(0xFF101A2E),
        borderRadius: BorderRadius.only(
          topLeft: Radius.circular(24),
          topRight: Radius.circular(24),
        ),
      ),
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(18, 18, 18, 12),
          child: Column(
            children: [
              Row(
                children: [
                  const Expanded(
                    child: Text(
                      'Admin Panel',
                      style: TextStyle(
                        color: Colors.white,
                        fontSize: 22,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  IconButton(
                    onPressed: () => Navigator.of(context).pop(),
                    icon: const Icon(Icons.close, color: Colors.white70),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.only(bottom: 12),
                  children: [
                    for (final node in nodes) ...[
                      Container(
                        margin: const EdgeInsets.only(bottom: 16),
                        padding: const EdgeInsets.all(14),
                        decoration: BoxDecoration(
                          color: const Color(0xFF121F32),
                          borderRadius: BorderRadius.circular(16),
                          border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                Container(
                                  width: 10,
                                  height: 10,
                                  decoration: BoxDecoration(
                                    shape: BoxShape.circle,
                                    color: node.active ? const Color(0xFFFF4D4D) : const Color(0xFF69778B),
                                  ),
                                ),
                                const SizedBox(width: 8),
                                Expanded(
                                  child: Text(
                                    node.name,
                                    style: const TextStyle(
                                      color: Colors.white,
                                      fontWeight: FontWeight.w700,
                                      fontSize: 17,
                                    ),
                                  ),
                                ),
                                Text(
                                  node.nodeType,
                                  style: const TextStyle(
                                    color: Color(0xFFA9B7C9),
                                    fontSize: 11,
                                    letterSpacing: 0.8,
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 12),
                            _InfoRow(label: 'Peer ID', value: node.peerId.toString()),
                            _InfoRow(label: 'IP', value: node.remoteAddr),
                            _InfoRow(label: 'Last heartbeat', value: node.lastHeartbeatAgo.isEmpty ? 'n/a' : node.lastHeartbeatAgo),
                            _InfoRow(label: 'RSSI', value: node.rssi == null ? 'n/a' : '${node.rssi!.toStringAsFixed(0)} dB'),
                            const SizedBox(height: 12),
                            if (node.config.isEmpty)
                              const Text(
                                'No config entries available',
                                style: TextStyle(color: Colors.white54),
                              )
                            else ...[
                              const Text(
                                'Config values',
                                style: TextStyle(
                                  color: Colors.white70,
                                  fontWeight: FontWeight.w600,
                                  fontSize: 12,
                                  letterSpacing: 0.7,
                                ),
                              ),
                              const SizedBox(height: 8),
                              for (final entry in node.config) ...[
                                Padding(
                                  padding: const EdgeInsets.only(bottom: 10),
                                  child: TextFormField(
                                    initialValue: entry.value,
                                    onChanged: (value) {
                                      final key = '${node.peerId}:${entry.key}:${entry.type}';
                                      _saveQueue[key] = value;
                                      _controllers[key] = TextEditingController(text: value);
                                    },
                                    style: const TextStyle(color: Colors.white),
                                    decoration: InputDecoration(
                                      labelText: '${entry.key} (${entry.type})',
                                      labelStyle: const TextStyle(color: Color(0xFFA9B7C9)),
                                      filled: true,
                                      fillColor: const Color(0xFF0D1728),
                                      border: OutlineInputBorder(
                                        borderRadius: BorderRadius.circular(12),
                                        borderSide: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
                                      ),
                                      enabledBorder: OutlineInputBorder(
                                        borderRadius: BorderRadius.circular(12),
                                        borderSide: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
                                      ),
                                      focusedBorder: OutlineInputBorder(
                                        borderRadius: BorderRadius.circular(12),
                                        borderSide: const BorderSide(color: Color(0xFF2DD4BF)),
                                      ),
                                    ),
                                  ),
                                ),
                              ],
                            ],
                          ],
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(height: 10),
              SizedBox(
                width: double.infinity,
                child: FilledButton.icon(
                  onPressed: () async {
                    final baseUrl = Uri.base.origin.isNotEmpty && Uri.base.origin != 'null'
                        ? Uri.base.origin
                        : 'http://localhost:8080';

                    final values = _saveQueue.entries.toList();
                    if (values.isEmpty) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(content: Text('No config values changed')),
                      );
                      return;
                    }

                    bool failed = false;
                    for (final entry in values) {
                      final parts = entry.key.split(':');
                      if (parts.length < 3) {
                        failed = true;
                        continue;
                      }
                      final peerId = int.tryParse(parts[0]) ?? 0;
                      final key = parts[1];
                      final type = parts[2];

                      final response = await http.post(
                        Uri.parse('$baseUrl/api/admin/config'),
                        headers: {
                          'Content-Type': 'application/json',
                          'Authorization': _basicAuthHeader(widget.username, widget.password),
                        },
                        body: jsonEncode({
                          'peerId': peerId,
                          'key': key,
                          'type': type,
                          'value': entry.value,
                        }),
                      );

                      if (response.statusCode != 200) {
                        failed = true;
                      }
                    }

                    if (!mounted) return;
                    if (failed) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(content: Text('One or more config changes failed')),
                      );
                    } else {
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(content: Text('Config saved successfully')),
                      );
                    }
                  },
                  icon: const Icon(Icons.save_outlined),
                  label: const Text('Save Changes'),
                  style: FilledButton.styleFrom(
                    padding: const EdgeInsets.symmetric(vertical: 16),
                    backgroundColor: const Color(0xFF2DD4BF),
                    foregroundColor: const Color(0xFF09111D),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
  String _basicAuthHeader(String username, String password) {
    final encoded = base64Encode(utf8.encode('$username:$password'));
    return 'Basic $encoded';
  }
}

class _InfoRow extends StatelessWidget {
  final String label;
  final String value;

  const _InfoRow({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 110,
            child: Text(
              label,
              style: const TextStyle(color: Colors.white54, fontSize: 12),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: const TextStyle(color: Colors.white, fontSize: 12),
            ),
          ),
        ],
      ),
    );
  }
}
