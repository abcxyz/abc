import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import 'screen/landing_screen.dart';
import 'screen/login_screen.dart';
import 'state/authentication/bloc/authentication_bloc.dart';

void main() {
  runApp(const FlutterApp());
}

class FlutterApp extends StatelessWidget {
  const FlutterApp({super.key});

  // This widget is the root of your application.
  @override
  Widget build(BuildContext context) {
    return BlocProvider<AuthenticationBloc>(
      create: (_) => AuthenticationBloc(),
      child: const AppDetails(),
    );
  }
}

class AppDetails extends StatelessWidget {
  const AppDetails({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      initialRoute: '/login',
      routes: <String, WidgetBuilder>{
        '/login': (_) => const LoginScreen(),
        '/landing': (_) => const LandingScreen(),
      },
      title: 'Flutter Template',
    );
  }
}
