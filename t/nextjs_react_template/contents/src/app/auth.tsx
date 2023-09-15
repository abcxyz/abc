'use client';

import { useUser } from '@auth0/nextjs-auth0/client';
import { Box, Button, Typography } from '@mui/material';
import { AUTH_LOADING_TEXT, AUTH_LOGIN_TEXT, AUTH_LOGOUT_TEXT } from './constants';

export default function Auth() {
  const { user, error, isLoading } = useUser();

  if (isLoading) return <Box>{AUTH_LOADING_TEXT}</Box>;
  if (error) return <Box>{error.message}</Box>;

  if (user) {
    return (
      <Box>
        <Typography variant="h5">Welcome {user.email}!</Typography>
        <Button color="inherit" href="/api/auth/logout">{AUTH_LOGOUT_TEXT}</Button>
      </Box>
    )
  }

  return <Button color="inherit" href="/api/auth/login">{AUTH_LOGIN_TEXT}</Button>;
}
