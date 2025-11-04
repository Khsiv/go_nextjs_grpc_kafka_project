import React from 'react';

interface Card {
  id: number;
  name: string;
  email: string;
  description?: string;
}

const CardComponent: React.FC<{ card: Card }> = ({ card }) => {
  return (
    <div className="bg-white shadow-lg rounded-lg p-2 mb-2 hover:bg-gray-100">
      <div className="text-sm text-gray-600">Id: {card.id}</div>
      <div className="text-lg font-semibold text-gray-800">{card.name}</div>
      <div className="text-md text-gray-700">{card.email}</div>
      {/* <div className="text-sm text-gray-700 mt-1">{card.description}</div> */}
      {card.description ? (
        <div className="text-sm text-gray-700 mt-1">{card.description}</div>
      ) : null}
    </div>
  );
};

export default CardComponent;