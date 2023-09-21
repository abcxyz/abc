'use client';

import {useUser} from '@auth0/nextjs-auth0/client';
import {
  Avatar,
  Box,
  Button,
  IconButton,
  Link,
  Menu,
  MenuItem,
} from '@mui/material';
import {
  AUTH_LOADING_TEXT,
  AUTH_LOGIN_TEXT,
  AUTH_LOGOUT_TEXT,
} from './constants';
import {useState} from 'react';

export default function Auth() {
  const {user, error, isLoading} = useUser();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  const handleMenu = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  if (isLoading) {
    return <Box>{AUTH_LOADING_TEXT}</Box>;
  }

  if (error) {
    return <Box>{error.message}</Box>;
  }

  if (user) {
    return (
      <Box sx={{flexGrow: 0}}>
        <IconButton sx={{p: 0}} onClick={handleMenu}>
          <Avatar alt="User Picture" src={user.picture ?? ''} />
        </IconButton>
        <Menu
          anchorEl={anchorEl}
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'left',
          }}
          keepMounted
          transformOrigin={{
            vertical: 'top',
            horizontal: 'center',
          }}
          open={Boolean(anchorEl)}
          onClose={handleClose}
        >
          <MenuItem component={Link} href="/api/auth/logout">
            {AUTH_LOGOUT_TEXT}
          </MenuItem>
        </Menu>
      </Box>
    );
  }

  return (
    <Button color="inherit" href="/api/auth/login">
      {AUTH_LOGIN_TEXT}
    </Button>
  );
}
