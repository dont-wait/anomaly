export interface User {
    id: string;
    name: string;
    email: string;
}

export interface Transaction {
    id: string;
    amount: number;
    type: "credit" | "debit";
    description: string;
    date: Date;
}
