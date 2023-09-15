'use client';

import { useUser } from '@auth0/nextjs-auth0/client';
import { Button } from '@mui/material';

export default function Auth() {
  const { user, error, isLoading } = useUser();

  if (isLoading) return <div>Loading...</div>;
  if (error) return <div>{error.message}</div>;

  if (user) {
    return (
      <div>
        Welcome {user.name}!
        <Button color="inherit" href="/api/auth/logout">Logout</Button>
      </div>
    )
  }

  return <Button color="inherit" href="/api/auth/login">Login</Button>;
}
