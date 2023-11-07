import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../state/authentication/bloc/authentication_bloc.dart';

class LoginScreen extends StatelessWidget {
  const LoginScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return BlocConsumer<AuthenticationBloc, AuthenticationState>(
      listener: (BuildContext context, AuthenticationState state) {
        if (state is AuthenticationError) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(state.message),
            ),
          );
        } else if (state is Authenticated) {
          Navigator.pushReplacementNamed(context, '/landing');
        }
      },
      builder: (BuildContext context, AuthenticationState state) {
        if (state is InitiateAuthentication) {
          return const Scaffold(
            body: Center(
              child: CircularProgressIndicator(),
            ),
          );
        }
        return Scaffold(
          body: Column(
            children: <Widget>[
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 100),
                child: Center(
                  child: Text(
                    'Welcome!',
                    style: TextStyle(fontSize: 28),
                  ),
                ),
              ),
              // Logo asset.
              Padding(
                padding: const EdgeInsets.all(20),
                child: Image.asset(
                  'assets/bp_logo.png',
                  height: 100,
                  fit: BoxFit.contain,
                ),
              ),
              Expanded(
                child: Container(),
              ),
              // Buttons
              ElevatedButton(
                onPressed: () async {
                  context.read<AuthenticationBloc>().add(AuthenticateUser());
                },
                child: const Text('Sign in'),
              ),
              Padding(
                padding: const EdgeInsets.only(bottom: 100),
                child: TextButton(
                  onPressed: () =>
                      Navigator.pushReplacementNamed(context, '/landing'),
                  child: const Text('Skip >'),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
