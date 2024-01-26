import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../state/authentication/bloc/authentication_bloc.dart';

class UserAvatar extends StatelessWidget {
  const UserAvatar({super.key});

  @override
  Widget build(BuildContext context) {
    return BlocBuilder<AuthenticationBloc, AuthenticationState>(
      builder: (BuildContext context, AuthenticationState state) {
        if (state is Authenticated && state.user.avatarUri != null) {
          return CircleAvatar(
            backgroundImage: NetworkImage(
              state.user.avatarUri!.toString(),
            ),
          );
        } else {
          return const CircleAvatar(
            child: Icon(
              Icons.account_circle,
            ),
          );
        }
      },
    );
  }
}
