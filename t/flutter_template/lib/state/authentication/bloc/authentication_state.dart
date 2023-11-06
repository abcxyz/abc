part of 'authentication_bloc.dart';

abstract class AuthenticationState {}

class Unauthenticated extends AuthenticationState {}

class InitiateAuthentication extends AuthenticationState {}

class Authenticated extends AuthenticationState {
  Authenticated(this.user);

  final UserModel user;
}

class AuthenticationError extends AuthenticationState {
  AuthenticationError(this.message);

  final String message;
}
