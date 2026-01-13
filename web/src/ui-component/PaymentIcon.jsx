import PropTypes from 'prop-types';
import { Box } from '@mui/material';

const PaymentIcon = ({ icon, size = 24, sx = {} }) => {
  if (!icon) {
    return null;
  }

  const isSvgContent = icon.trim().startsWith('<svg');

  if (isSvgContent) {
    return (
      <Box
        sx={{
          width: size,
          height: size,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          '& svg': {
            width: '100%',
            height: '100%'
          },
          ...sx
        }}
        dangerouslySetInnerHTML={{ __html: icon }}
      />
    );
  }

  return (
    <Box
      component="img"
      src={icon}
      alt="payment icon"
      sx={{
        width: size,
        height: size,
        objectFit: 'contain',
        ...sx
      }}
    />
  );
};

PaymentIcon.propTypes = {
  icon: PropTypes.string,
  size: PropTypes.number,
  sx: PropTypes.object
};

export default PaymentIcon;
