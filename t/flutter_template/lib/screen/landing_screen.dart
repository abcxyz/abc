import 'package:flutter/material.dart';

import '../component/user_avatar.dart';

class LandingScreen extends StatelessWidget {
  const LandingScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Flutter Template'),
        actions: const <Widget>[
          UserAvatar(),
          SizedBox(width: 10),
        ],
      ),
      body: Container(),
    );
  }
}
