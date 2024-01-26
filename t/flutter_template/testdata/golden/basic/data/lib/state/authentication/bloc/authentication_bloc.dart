import 'package:auth0_flutter/auth0_flutter.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../model/user_model.dart';

part 'authentication_event.dart';
part 'authentication_state.dart';

class AuthenticationBloc
    extends Bloc<AuthenticationEvent, AuthenticationState> {
  AuthenticationBloc() : super(Unauthenticated()) {
    on<AuthenticateUser>(
        (AuthenticateUser event, Emitter<AuthenticationState> emit) async {
      try {
        emit(InitiateAuthentication());
        final Credentials credentials =
            await auth0.webAuthentication(scheme: scheme).login();
        emit(
          Authenticated(
            UserModel(
              credentials.user.name!,
              credentials.user.email!,
              credentials.user.pictureUrl,
            ),
          ),
        );
      } on Exception catch (e, _) {
        emit(AuthenticationError(e.toString()));
      }
    });
  }

  final Auth0 auth0 = Auth0(
    'example.com',
    'my-client-id',
  );
  final String scheme = 'https://my-domain';
}
