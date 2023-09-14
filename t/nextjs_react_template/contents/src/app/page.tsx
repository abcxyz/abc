import Box from '@mui/material/Box';
import Image from 'next/image';

export default function Home() {
  return (
    <Box
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      minHeight="100vh"
    >
      <Image
        src="/bets-platform-logo.png"
        alt="bets-platform"
        height={200}
        width={200}
      />
    </Box>
  );
}
