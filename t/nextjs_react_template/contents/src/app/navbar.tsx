import {AppBar, Toolbar, Typography} from '@mui/material';
import Auth from './auth';
import {APP_TITLE} from './constants';

export default function NavBar() {
  return (
    <AppBar position="static">
      <Toolbar>
        <Typography
          variant="h5"
          component="div"
          fontWeight="bold"
          sx={{flexGrow: 1}}
        >
          {APP_TITLE}
        </Typography>
        <Auth />
      </Toolbar>
    </AppBar>
  );
}
