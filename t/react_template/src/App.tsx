// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import Typography from '@mui/material/Typography';
import Grid from '@mui/material/Grid';

const logo = require('./img/bets-platform-logo.png');

const App = () => {
  return (
    <Grid
      container
      spacing={0}
      direction="column"
      alignItems="center"
      textAlign="center"
      justifyContent="center"
      sx={{minHeight: '100vh'}}
    >
      <Grid item xs={3}>
        <img src={logo} alt="bets-platform" height={200} />
        <Typography variant="h4" textAlign="center">
          hellow world
        </Typography>
      </Grid>
    </Grid>
  );
};

export default App;
