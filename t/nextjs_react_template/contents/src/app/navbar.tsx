import { AppBar, Toolbar, Typography } from "@mui/material";
import Auth from "./auth";
import { APP_TITLE } from "./constants";

export default function NavBar() {
  return (
    <AppBar position="static" sx={{ bgcolor: "#202124" }}>
      <Toolbar>
        <Typography
          variant="h5"
          component="div"
          fontWeight={700}
          sx={{ flexGrow: 1 }}
        >
          {APP_TITLE}
        </Typography>
        <Auth />
      </Toolbar>
    </AppBar>
  )
}
