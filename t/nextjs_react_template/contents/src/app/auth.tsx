'use client';

import { useUser } from '@auth0/nextjs-auth0/client';
import { Button } from '@mui/material';
import { AUTH_LOADING_TEXT, AUTH_LOGIN_TEXT, AUTH_LOGOUT_TEXT } from './constants';

export default function Auth() {
  const { user, error, isLoading } = useUser();

  if (isLoading) return <div>{AUTH_LOADING_TEXT}</div>;
  if (error) return <div>{error.message}</div>;

  if (user) {
    return (
      <div>
        Welcome {user.email}!
        <Button color="inherit" href="/api/auth/logout">{AUTH_LOGOUT_TEXT}</Button>
      </div>
    )
  }

  return <Button color="inherit" href="/api/auth/login">{AUTH_LOGIN_TEXT}</Button>;
}
